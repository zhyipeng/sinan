package hostupdate

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"sinan/internal/buildinfo"
)

type Plan struct {
	Target   string `json:"target"`
	Work     string `json:"work"`
	Task     string `json:"task"`
	Version  string `json:"version"`
	Previous string `json:"previous"`
	Parent   int    `json:"parent"`
}

var sessionPattern = regexp.MustCompile(`^install-[A-Za-z0-9]+$`)

func CacheRoot() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", errors.New("无法确定更新缓存目录")
	}
	root = filepath.Join(root, "Sinan", "updates")
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root, nil
}
func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
func taskDir(session string) (string, error) {
	if !sessionPattern.MatchString(session) {
		return "", errors.New("更新会话无效")
	}
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, session), nil
}
func (p Plan) Stage() string  { return filepath.Join(p.Work, ArchiveRoot()) }
func (p Plan) Backup() string { return filepath.Join(p.Work, "previous") }
func (p Plan) validate() error {
	if p.Target == p.Work || p.Target == p.Task || strings.HasPrefix(p.Task, p.Target+string(os.PathSeparator)) {
		return errors.New("更新路径重叠")
	}
	task, err := taskDir(filepath.Base(p.Task))
	if err != nil || task != p.Task || !filepath.IsAbs(p.Target) || !filepath.IsAbs(p.Work) || filepath.Clean(p.Target) != p.Target || filepath.Dir(p.Target) == p.Target || filepath.Dir(p.Work) != filepath.Dir(p.Target) || !regexp.MustCompile(`^\.sinan-update-[0-9]+$`).MatchString(filepath.Base(p.Work)) || !buildinfo.ValidVersion(p.Version) || !buildinfo.ValidVersion(p.Previous) || buildinfo.Compare(p.Version, p.Previous) <= 0 || p.Parent <= 0 {
		return errors.New("更新计划无效")
	}
	for _, directory := range []string{p.Target, p.Work, p.Task} {
		resolved, err := filepath.EvalSymlinks(directory)
		if err != nil || resolved != directory {
			return errors.New("更新路径已变化或含有链接")
		}
	}
	if err := ValidatePackage(p.Target, p.Previous); err != nil {
		return err
	}
	return ValidatePackage(p.Stage(), p.Version)
}

// StageArchive extracts beside the installation so both renames stay on one volume.
func StageArchive(archive, target, version, previous string) (Plan, error) {
	p := Plan{Target: target, Version: version, Previous: previous, Parent: os.Getpid()}
	root, err := CacheRoot()
	if err != nil {
		return p, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return p, errors.New("无法创建更新缓存")
	}
	p.Task, err = os.MkdirTemp(root, "install-")
	if err != nil {
		return p, err
	}
	// Normalize macOS /var aliases before persisting paths.
	p.Task, err = filepath.EvalSymlinks(p.Task)
	if err != nil {
		return p, err
	}
	p.Work, err = os.MkdirTemp(filepath.Dir(target), ".sinan-update-")
	if err != nil {
		os.RemoveAll(p.Task)
		return p, errors.New("应用目录不可写，请移动至可写目录后重试")
	}
	good := false
	defer func() {
		if !good {
			p.Cleanup()
		}
	}()
	if err = Extract(archive, p.Work, runtime.GOOS == "windows"); err != nil {
		return p, err
	}
	if err = p.validate(); err != nil {
		return p, err
	}
	if runtime.GOOS == "darwin" {
		if exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", p.Stage()).Run() != nil {
			return p, errors.New("新版应用签名校验失败")
		}
	}
	if err = writeJSON(filepath.Join(p.Task, "plan.json"), p); err != nil {
		return p, err
	}
	good = true
	return p, nil
}
func (p Plan) Cleanup() {
	if p.Work != "" {
		os.RemoveAll(p.Work)
	}
	if p.Task != "" {
		os.RemoveAll(p.Task)
	}
}

// Handoff copies the current trusted helper outside the installation and waits
// for acknowledgement before allowing the caller to terminate the desktop host.
func Handoff(p Plan) error {
	if err := p.validate(); err != nil {
		return err
	}
	source, err := os.Open(filepath.Join(BinaryDir(p.Target), "sinan-updater"+suffix()))
	if err != nil {
		return errors.New("更新助手不可用")
	}
	defer source.Close()
	helper := filepath.Join(p.Task, "sinan-updater"+suffix())
	output, err := os.OpenFile(helper, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, source)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		return errors.New("无法准备更新助手")
	}
	cmd := exec.Command(helper, filepath.Join(p.Task, "plan.json"))
	cmd.Dir = p.Task
	configureProcess(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return errors.New("无法启动更新助手")
	}
	ready := make(chan bool, 1)
	go func() {
		line, err := bufio.NewReader(io.LimitReader(stdout, 32)).ReadString('\n')
		ready <- err == nil && line == "READY\n"
	}()
	select {
	case ok := <-ready:
		if !ok {
			cmd.Process.Kill()
			cmd.Wait()
			return errors.New("更新助手拒绝计划，应用未退出")
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		return errors.New("更新助手未就绪，应用未退出")
	}
	go cmd.Wait()
	return nil
}

// ReadPlan is used by the helper before acknowledging the host.
func ReadPlan(path string) (Plan, error) {
	var p Plan
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
		return p, errors.New("更新计划文件无效")
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &p) != nil || path != filepath.Join(p.Task, "plan.json") {
		return p, errors.New("更新计划文件无效")
	}
	return p, p.validate()
}
func ConfirmStartup(session, version, executable string) error {
	dir, err := taskDir(session)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, "plan.json"))
	if err != nil {
		return err
	}
	var p Plan
	if json.Unmarshal(data, &p) != nil || p.Task != dir || p.Version != version {
		return errors.New("更新健康检查会话不符")
	}
	target, err := Installation(executable, version)
	if err != nil || target != p.Target {
		return errors.New("更新健康检查路径不符")
	}
	return os.WriteFile(filepath.Join(dir, "healthy"), []byte(version), 0600)
}
func renameRetry(source, target string) error {
	var err error
	for i := 0; i < 40; i++ {
		err = os.Rename(source, target)
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// ReplaceAndRestart keeps the complete old directory. launch must return true
// only after a fresh application-started acknowledgement from the new host.
func ReplaceAndRestart(p Plan, launch func(string, string) bool) error {
	if err := p.validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(p.Backup()); !errors.Is(err, os.ErrNotExist) {
		return errors.New("回退目录已存在，拒绝覆盖")
	}
	if err := renameRetry(p.Target, p.Backup()); err != nil {
		return errors.New("应用仍被占用或目录不可写，未替换")
	}
	if err := renameRetry(p.Stage(), p.Target); err != nil {
		if renameRetry(p.Backup(), p.Target) != nil {
			return errors.New("新版替换失败，自动回退失败；旧版保留在更新暂存目录")
		}
		return errors.New("新版替换失败，已恢复旧版")
	}
	if launch(filepath.Join(BinaryDir(p.Target), "sinan"+suffix()), filepath.Base(p.Task)) {
		return nil
	}
	if renameRetry(p.Target, p.Stage()) != nil || renameRetry(p.Backup(), p.Target) != nil {
		return errors.New("新版未能启动，自动回退失败；旧版保留在更新暂存目录")
	}
	launch(filepath.Join(BinaryDir(p.Target), "sinan"+suffix()), "")
	return errors.New("新版未能启动，已回退旧版")
}
func Launch(executable, session string) bool {
	args := []string{}
	if session != "" {
		args = []string{"-update-session", session}
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = filepath.Dir(executable)
	configureProcess(cmd)
	if cmd.Start() != nil {
		return false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if session == "" {
		return true
	}
	task, err := taskDir(session)
	if err != nil {
		cmd.Process.Kill()
		<-done
		return false
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ticker.C:
			data, err := os.ReadFile(filepath.Join(task, "healthy"))
			if err == nil {
				var p Plan
				raw, _ := os.ReadFile(filepath.Join(task, "plan.json"))
				if json.Unmarshal(raw, &p) == nil && string(data) == p.Version {
					return true
				}
			}
		case <-done:
			return false
		case <-deadline.C:
			cmd.Process.Kill()
			<-done
			return false
		}
	}
}
func SaveResult(message string) {
	root, err := CacheRoot()
	if err == nil {
		_ = writeJSON(filepath.Join(root, "last-result.json"), struct {
			Message string `json:"message"`
		}{message})
	}
}
func LastResult() string {
	root, err := CacheRoot()
	if err != nil {
		return ""
	}
	info, err := os.Stat(filepath.Join(root, "last-result.json"))
	if err != nil || info.Size() > 4096 {
		return ""
	}
	data, _ := os.ReadFile(filepath.Join(root, "last-result.json"))
	var result struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &result)
	return result.Message
}
