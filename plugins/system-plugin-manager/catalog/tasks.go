package catalog

import (
	"context"
	"errors"
	"fmt"
	"sinan/internal/kernel"
	"sync"
	"time"
)

// InstallTask is a host-owned operation. It contains no preview ticket or paths.
type InstallTask struct {
	DownloadedBytes int64  `json:"downloadedBytes"`
	TotalBytes      int64  `json:"totalBytes"`
	ID              string `json:"id"`
	PluginID        string `json:"pluginId"`
	Version         string `json:"version"`
	Revision        string `json:"revision"`
	State           string `json:"state"`
	Error           string `json:"error"`
}
type installWork struct {
	task   InstallTask
	cancel context.CancelFunc
}
type installTasks struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	closed   bool
	sequence uint64
	work     []*installWork
}

func activeTask(state string) bool { return state == "downloading" || state == "installing" }

// StartInstall deliberately does not inherit the view's request context.
// The click authorizes native installation just as the synchronous UI did.
func (s *Service) StartInstall(id, version, revision string, installed func(kernel.PluginInfo)) (InstallTask, error) {
	t := &s.tasks
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return InstallTask{}, errors.New("商店已关闭")
	}
	count := 0
	for _, w := range t.work {
		if activeTask(w.task.State) {
			if w.task.PluginID == id {
				return w.task, nil
			}
			count++
		}
	}
	if count >= 4 {
		return InstallTask{}, errors.New("最多同时下载四个插件，请稍后重试")
	}
	// Keep a bounded recent history, preserving active tasks.
	if len(t.work) >= 32 {
		for i, w := range t.work {
			if !activeTask(w.task.State) {
				t.work = append(t.work[:i], t.work[i+1:]...)
				break
			}
		}
	}
	t.sequence++
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	w := &installWork{task: InstallTask{ID: fmt.Sprint(t.sequence), PluginID: id, Version: version, Revision: revision, State: "downloading"}, cancel: cancel}
	t.work = append(t.work, w)
	t.wg.Add(1)
	go func() { defer t.wg.Done(); defer cancel(); s.runInstall(ctx, w, installed) }()
	return w.task, nil
}
func (s *Service) runInstall(ctx context.Context, w *installWork, installed func(kernel.PluginInfo)) {
	preview, err := s.prepare(ctx, w.task.PluginID, w.task.Version, w.task.Revision, func(downloaded, total int64) {
		s.tasks.mu.Lock()
		defer s.tasks.mu.Unlock()
		w.task.DownloadedBytes = downloaded
		w.task.TotalBytes = total
	})
	if err == nil {
		defer s.Cancel(preview.Ticket)
		s.tasks.mu.Lock()
		err = ctx.Err()
		if err == nil {
			w.task.State = "installing"
		}
		s.tasks.mu.Unlock()
		if err == nil && preview.Changed {
			var result kernel.PluginInfo
			result, err = s.Apply(ctx, preview.Ticket, preview.Kind == "process")
			if err == nil && installed != nil {
				installed(result)
			}
		}
	}
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	if err == nil {
		w.task.State = "succeeded"
	} else if errors.Is(ctx.Err(), context.Canceled) {
		w.task.State = "cancelled"
	} else {
		w.task.State = "failed"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			w.task.Error = "下载安装超时，请重试"
		} else {
			w.task.Error = err.Error()
		}
	}
}
func (s *Service) InstallTasks() []InstallTask {
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	result := make([]InstallTask, 0, len(s.tasks.work))
	for _, w := range s.tasks.work {
		result = append(result, w.task)
	}
	return result
}
func (s *Service) CancelInstall(id string) error {
	s.tasks.mu.Lock()
	defer s.tasks.mu.Unlock()
	for _, w := range s.tasks.work {
		if w.task.ID == id {
			if w.task.State == "installing" {
				return errors.New("正在应用安装，无法取消")
			}
			if w.task.State == "downloading" {
				w.cancel()
			}
			return nil
		}
	}
	return errors.New("安装任务不存在")
}
func (s *Service) closeInstallTasks() {
	s.tasks.mu.Lock()
	s.tasks.closed = true
	for _, w := range s.tasks.work {
		w.cancel()
	}
	s.tasks.mu.Unlock()
	s.tasks.wg.Wait()
}
