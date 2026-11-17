package catalog

import (
	"context"
	"sinan/internal/kernel"
	"sinan/internal/plugin/distribution"
	"sync/atomic"
	"testing"
	"time"
)

type gatedStager struct {
	directory string
	started   chan struct{}
	release   chan struct{}
	cleaned   atomic.Int32
}

func (g *gatedStager) Stage(ctx context.Context, _ string, _ distribution.Artifact) (string, func(), error) {
	close(g.started)
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-g.release:
		return g.directory, func() { g.cleaned.Add(1) }, nil
	}
}
func (g *gatedStager) StageWithProgress(ctx context.Context, cache string, artifact distribution.Artifact, progress func(int64, int64)) (string, func(), error) {
	progress(0, artifact.Size)
	progress(artifact.Size/2, artifact.Size)
	directory, cleanup, err := g.Stage(ctx, cache, artifact)
	if err == nil {
		progress(artifact.Size, artifact.Size)
	}
	return directory, cleanup, err
}
func taskFixture(t *testing.T) (*Service, *gatedStager, *kernel.Manager) {
	t.Helper()
	s, _, f, m := serviceFixture(t)
	g := &gatedStager{directory: f.directory, started: make(chan struct{}), release: make(chan struct{})}
	s.Fetcher = g
	return s, g, m
}
func awaitTask(t *testing.T, s *Service, state string) InstallTask {
	t.Helper()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		tasks := s.InstallTasks()
		if len(tasks) == 1 && tasks[0].State == state {
			return tasks[0]
		}
		select {
		case <-deadline:
			t.Fatalf("waiting for %s: %+v", state, tasks)
		case <-ticker.C:
		}
	}
}
func TestInstallTaskReturnsBeforeDownloadAndInstallsWithoutView(t *testing.T) {
	s, g, m := taskFixture(t)
	callback := make(chan struct{}, 1)
	task, err := s.StartInstall("example.store", "0.1.0", "reviewed", func(kernel.PluginInfo) { callback <- struct{}{} })
	if err != nil || task.State != "downloading" {
		t.Fatal(task, err)
	}
	<-g.started
	snapshot := s.InstallTasks()[0]
	if snapshot.DownloadedBytes != 50 || snapshot.TotalBytes != 100 {
		t.Fatal("download progress not retained", snapshot)
	}
	duplicate, err := s.StartInstall("example.store", "0.1.0", "reviewed", nil)
	if err != nil || duplicate.ID != task.ID || len(s.InstallTasks()) != 1 {
		t.Fatal("duplicate task", duplicate, err)
	}
	// No client response or mounted view is needed to finish the operation.
	close(g.release)
	finished := awaitTask(t, s, "succeeded")
	if finished.DownloadedBytes != 100 || finished.TotalBytes != 100 {
		t.Fatal("final download progress missing", finished)
	}
	s.Close()
	if len(m.Snapshot().Plugins) != 1 || g.cleaned.Load() != 1 {
		t.Fatal("install or cleanup missing")
	}
	select {
	case <-callback:
	default:
		t.Fatal("installed callback missing")
	}
}
func TestInstallTaskCancelAndRetry(t *testing.T) {
	s, g, m := taskFixture(t)
	task, err := s.StartInstall("example.store", "0.1.0", "reviewed", nil)
	if err != nil {
		t.Fatal(err)
	}
	<-g.started
	if err = s.CancelInstall(task.ID); err != nil {
		t.Fatal(err)
	}
	awaitTask(t, s, "cancelled")
	if len(m.Snapshot().Plugins) != 0 {
		t.Fatal("cancelled task installed")
	}
	// The terminal task must not prevent an explicit retry.
	next := &gatedStager{directory: g.directory, started: make(chan struct{}), release: make(chan struct{})}
	// Wait for the previous worker before replacing the test dependency.
	s.tasks.wg.Wait()
	s.Fetcher = next
	close(next.release)
	retry, err := s.StartInstall("example.store", "0.1.0", "reviewed", nil)
	if err != nil || retry.ID == task.ID {
		t.Fatal(retry, err)
	}
	s.tasks.wg.Wait()
	if s.InstallTasks()[1].State != "succeeded" {
		t.Fatal(s.InstallTasks())
	}
}
func TestInstallTaskShutdownCancelsAndJoinsDownload(t *testing.T) {
	s, g, m := taskFixture(t)
	if _, err := s.StartInstall("example.store", "0.1.0", "reviewed", nil); err != nil {
		t.Fatal(err)
	}
	<-g.started
	s.Close()
	awaitTask(t, s, "cancelled")
	if len(m.Snapshot().Plugins) != 0 {
		t.Fatal("shutdown task installed")
	}
	if _, err := s.StartInstall("example.store", "0.1.0", "reviewed", nil); err == nil {
		t.Fatal("accepted work after close")
	}
}

type gatedBackend struct {
	Backend
	applying chan struct{}
	release  chan struct{}
}

func (b *gatedBackend) ApplyPackage(path string, trust bool, fingerprint, installedFingerprint string) (kernel.PluginInfo, error) {
	close(b.applying)
	<-b.release
	return b.Backend.ApplyPackage(path, trust, fingerprint, installedFingerprint)
}
func TestInstallTaskCannotCancelCommitAndShutdownWaits(t *testing.T) {
	s, g, m := taskFixture(t)
	b := &gatedBackend{Backend: m, applying: make(chan struct{}), release: make(chan struct{})}
	s.Backend = b
	close(g.release)
	task, err := s.StartInstall("example.store", "0.1.0", "reviewed", nil)
	if err != nil {
		t.Fatal(err)
	}
	<-b.applying
	if err = s.CancelInstall(task.ID); err == nil {
		t.Fatal("cancelled commit")
	}
	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("shutdown returned before commit finished")
	default:
	}
	close(b.release)
	<-closed
	awaitTask(t, s, "succeeded")
	if len(m.Snapshot().Plugins) != 1 || g.cleaned.Load() != 1 {
		t.Fatal("commit or cleanup missing")
	}
}

func TestInstallTaskReportsValidationFailure(t *testing.T) {
	s, g, m := taskFixture(t)
	close(g.release)
	// A stale revision fails asynchronously and never reaches apply.
	if _, err := s.StartInstall("example.store", "0.1.0", "old-revision", nil); err != nil {
		t.Fatal(err)
	}
	task := awaitTask(t, s, "failed")
	s.tasks.wg.Wait()
	if task.Error == "" || len(m.Snapshot().Plugins) != 0 {
		t.Fatal(task)
	}
}

type cancelLoader struct{}

func (cancelLoader) Load(ctx context.Context, _ bool) (Snapshot, error) {
	<-ctx.Done()
	return Snapshot{}, ctx.Err()
}
func TestInstallTaskBoundsConcurrentDownloads(t *testing.T) {
	s, _, _, _ := serviceFixture(t)
	s.Loader = cancelLoader{}
	for _, id := range []string{"one", "two", "three", "four"} {
		if _, err := s.StartInstall(id, "0.1.0", "reviewed", nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.StartInstall("five", "0.1.0", "reviewed", nil); err == nil {
		t.Fatal("unbounded downloads")
	}
	if _, err := s.StartInstall("one", "0.1.0", "reviewed", nil); err != nil {
		t.Fatal("duplicate should reuse task", err)
	}
	s.Close()
	for _, task := range s.InstallTasks() {
		if task.State != "cancelled" {
			t.Fatal(task)
		}
	}
}
