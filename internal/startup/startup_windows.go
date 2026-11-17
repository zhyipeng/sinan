package startup

import (
	"errors"
	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const valueName = "Sinan"

type windowsRegistration struct{ command string }

func newRegistration(executable string) (Registration, error) {
	return windowsRegistration{`"` + executable + `"`}, nil
}
func (r windowsRegistration) Enabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("无法读取开机自启配置")
	}
	defer k.Close()
	value, _, err := k.GetStringValue(valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("无法读取开机自启配置")
	}
	return value == r.command, nil
}
func (r windowsRegistration) SetEnabled(enabled bool) error {
	if !enabled {
		k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return errors.New("无法修改开机自启配置")
		}
		defer k.Close()
		if err = k.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return errors.New("无法移除开机自启配置")
		}
		return nil
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return errors.New("无法修改开机自启配置")
	}
	defer k.Close()
	if k.SetStringValue(valueName, r.command) != nil {
		return errors.New("无法保存开机自启配置")
	}
	return nil
}
