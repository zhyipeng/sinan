package main

import (
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"sinan/internal/storage"
)

type shortcutRegistry interface {
	Register(string, func()) error
	Unregister(string) error
}
type shortcutPreferences interface {
	LauncherPreferences() (storage.SettingsSnapshot, error)
	SaveLauncherPreferences(uint64, string) (storage.SettingsSnapshot, error)
}
type ShortcutState struct {
	Shortcut string `json:"shortcut"`
	Active   string `json:"active"`
	Default  string `json:"default"`
	Revision uint64 `json:"revision"`
	Error    string `json:"error"`
	Ready    bool   `json:"ready"`
}
type shortcutController struct {
	mu             sync.Mutex
	notifyMu       sync.Mutex
	registry       shortcutRegistry
	preferences    shortcutPreferences
	state          ShortcutState
	recordingUntil time.Time
	toggle         func()
	captured       func(string)
	changed        func(ShortcutState)
}

func defaultShortcut() string {
	if runtime.GOOS == "darwin" {
		return "Cmd+Space"
	}
	return "Ctrl+Space"
}
func normalizeShortcut(value string) (string, error) {
	if len(value) > 128 {
		return "", errors.New("快捷键格式无效")
	}
	parts := strings.Split(value, "+")
	if len(parts) < 2 || len(parts) > 5 {
		return "", errors.New("请同时按下 Command、Ctrl 或 Alt 与一个按键")
	}
	mods := map[string]bool{}
	for _, part := range parts[:len(parts)-1] {
		mod := ""
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "ctrl", "control":
			mod = "Ctrl"
		case "alt", "option":
			mod = "Alt"
		case "shift":
			mod = "Shift"
		case "cmd", "command", "cmdorctrl":
			if runtime.GOOS == "darwin" {
				mod = "Cmd"
			} else {
				mod = "Ctrl"
			}
		case "super", "win":
			if runtime.GOOS == "darwin" {
				mod = "Cmd"
			} else {
				mod = "Super"
			}
		default:
			return "", errors.New("快捷键包含不支持的修饰键")
		}
		if mods[mod] {
			return "", errors.New("快捷键包含重复修饰键")
		}
		mods[mod] = true
	}
	if !mods["Cmd"] && !mods["Ctrl"] && !mods["Alt"] && !mods["Super"] {
		return "", errors.New("请加入 Command、Ctrl 或 Alt")
	}
	key := strings.ToUpper(strings.TrimSpace(parts[len(parts)-1]))
	named := map[string]string{"SPACE": "Space", "ENTER": "Enter", "RETURN": "Enter", "TAB": "Tab", "BACKSPACE": "Backspace", "DELETE": "Delete", "HOME": "Home", "END": "End", "PAGE UP": "Page Up", "PAGE DOWN": "Page Down", "UP": "Up", "DOWN": "Down", "LEFT": "Left", "RIGHT": "Right"}
	if canonical, ok := named[key]; ok {
		key = canonical
	} else if len(key) == 1 && ((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= '0' && key[0] <= '9')) {
	} else if len(key) >= 2 && key[0] == 'F' {
		number, err := strconv.Atoi(key[1:])
		if err != nil || number < 1 || number > 20 {
			return "", errors.New("不支持此按键")
		}
		key = "F" + strconv.Itoa(number)
	} else {
		return "", errors.New("请使用字母、数字、空格、方向键或 F1–F20 等标准按键")
	}
	result := []string{}
	for _, mod := range []string{"Ctrl", "Alt", "Shift", "Cmd", "Super"} {
		if mods[mod] {
			result = append(result, mod)
		}
	}
	return strings.Join(append(result, key), "+"), nil
}
func newShortcutController(preferences shortcutPreferences, registry shortcutRegistry, toggle func(), captured func(string), changed func(ShortcutState)) *shortcutController {
	return newShortcutControllerWithDefault(preferences, registry, toggle, captured, changed, defaultShortcut())
}
func newShortcutControllerWithDefault(preferences shortcutPreferences, registry shortcutRegistry, toggle func(), captured func(string), changed func(ShortcutState), fallback string) *shortcutController {
	c := &shortcutController{registry: registry, preferences: preferences, toggle: toggle, captured: captured, changed: changed, state: ShortcutState{Default: fallback, Shortcut: fallback}}
	saved, err := preferences.LauncherPreferences()
	if err != nil {
		c.state.Error = "无法读取快捷键配置，暂用默认快捷键。"
		return c
	}
	c.state.Revision = saved.Revision
	if values := saved.Values["shortcut"]; len(values) > 0 {
		value, err := normalizeShortcut(values[0])
		if err != nil {
			c.state.Error = "已保存的快捷键无效，暂用默认快捷键。"
		} else {
			c.state.Shortcut = value
		}
	}
	return c
}
func (c *shortcutController) callback(key string) func() { return func() { go c.pressed(key) } }
func (c *shortcutController) pressed(key string) {
	c.mu.Lock()
	if key != c.state.Active {
		c.mu.Unlock()
		return
	}
	recording := time.Now().Before(c.recordingUntil)
	c.mu.Unlock()
	if recording {
		if c.captured != nil {
			c.captured(key)
		}
	} else {
		c.toggle()
	}
}
func (c *shortcutController) publish() {
	if c.changed == nil {
		return
	}
	go func() { c.notifyMu.Lock(); defer c.notifyMu.Unlock(); c.changed(c.State()) }()
}
func (c *shortcutController) Start() {
	c.mu.Lock()
	if c.state.Ready {
		c.mu.Unlock()
		return
	}
	c.state.Ready = true
	if err := c.registry.Register(c.state.Shortcut, c.callback(c.state.Shortcut)); err != nil {
		c.state.Error = "快捷键注册失败，请修改组合键或从托盘打开。"
	} else {
		c.state.Active = c.state.Shortcut
	}
	c.mu.Unlock()
	c.publish()
}
func (c *shortcutController) State() ShortcutState { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *shortcutController) Record(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if enabled && !c.state.Ready {
		return errors.New("快捷键尚未就绪，请稍后重试")
	}
	if enabled {
		c.recordingUntil = time.Now().Add(30 * time.Second)
	} else {
		c.recordingUntil = time.Time{}
	}
	return nil
}
func (c *shortcutController) Save(value string, revision uint64) (ShortcutState, error) {
	value, err := normalizeShortcut(value)
	if err != nil {
		return ShortcutState{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.state.Ready {
		return ShortcutState{}, errors.New("快捷键尚未就绪，请稍后重试")
	}
	if revision != c.state.Revision {
		return ShortcutState{}, errors.New("快捷键已变化，请重新打开设置后重试")
	}
	old := c.state.Active
	if value == old && value == c.state.Shortcut && c.state.Error == "" {
		return c.state, nil
	}
	if value != old {
		if err = c.registry.Register(value, c.callback(value)); err != nil {
			return ShortcutState{}, errors.New("快捷键注册失败，可能已被系统或其他应用占用；原快捷键保持不变。")
		}
		if old != "" {
			if err = c.registry.Unregister(old); err != nil {
				_ = c.registry.Unregister(value)
				// Wails removes the old registry entry before the OS operation returns.
				if restoreErr := c.registry.Register(old, c.callback(old)); restoreErr != nil {
					c.state.Active = ""
					c.state.Error = "快捷键恢复失败，请从托盘打开设置后重试。"
					c.publish()
				}
				return ShortcutState{}, errors.New("无法释放原快捷键，修改未保存，请重试。")
			}
		}
	}
	saved, err := c.preferences.SaveLauncherPreferences(revision, value)
	if err != nil {
		if value != old {
			_ = c.registry.Unregister(value)
			if old != "" {
				if restoreErr := c.registry.Register(old, c.callback(old)); restoreErr != nil {
					c.state.Active = ""
					c.state.Error = "快捷键恢复失败，请从托盘打开设置后重试。"
					c.publish()
				}
			}
		}
		return ShortcutState{}, errors.New("快捷键保存失败，修改未应用，请检查本地存储后重试。")
	}
	c.state.Shortcut = value
	c.state.Active = value
	c.state.Revision = saved.Revision
	c.state.Error = ""
	c.recordingUntil = time.Time{}
	c.publish()
	return c.state, nil
}
