package uiprocess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"sinan/internal/plugin/uiview"
)

type Options struct {
	Panel        bool
	WindowAction func(string) bool // signal state only; never block on UI

	Executable     string
	Args           []string
	StartupTimeout time.Duration
	Lifetime       time.Duration
	ShutdownGrace  time.Duration
	// Fixed development probes only. No such operation is enabled by default.
	ProbeTarget  string
	ProbeDisable func() error
}
type Result struct {
	Report json.RawMessage
	Err    error
}
type Process struct {
	focusSequence uint64
	focusPending  map[uint64]chan bool
	cmd           *exec.Cmd
	input         *os.File
	output        *os.File
	session       *uiview.Session
	options       Options
	exchange      sync.Mutex
	mu            sync.Mutex
	result        Result
	finished      bool
	ready         chan struct{}
	readyOnce     sync.Once
	stop          chan struct{}
	stopOnce      sync.Once
	done          chan struct{}
	readDone      chan struct{}
	unsubscribe   func()
}

func Start(ctx context.Context, session *uiview.Session, options Options) (*Process, error) {
	if session == nil {
		return nil, errors.New("UI 会话不存在")
	}
	failed := true
	defer func() {
		if failed {
			session.Revoke()
		}
	}()
	if !session.Active() || ctx.Err() != nil {
		return nil, errors.New("UI 会话不可用")
	}
	if !filepath.IsAbs(options.Executable) {
		return nil, errors.New("UI 客体必须使用宿主指定的绝对路径")
	}
	if options.StartupTimeout <= 0 {
		options.StartupTimeout = 3 * time.Second
	}
	if options.Lifetime <= 0 {
		options.Lifetime = 30 * time.Second
	}
	if options.ShutdownGrace <= 0 {
		options.ShutdownGrace = 500 * time.Millisecond
	}
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		return nil, errors.New("无法创建 UI 管道")
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		inRead.Close()
		inWrite.Close()
		return nil, errors.New("无法创建 UI 管道")
	}
	cmd := exec.Command(options.Executable, options.Args...)
	cmd.Dir = filepath.Dir(options.Executable)
	cmd.Env = []string{"LANG=en_US.UTF-8"}
	cmd.Stdin = inRead
	cmd.Stdout = outWrite
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		inRead.Close()
		inWrite.Close()
		outRead.Close()
		outWrite.Close()
		return nil, errors.New("无法启动 UI 客体")
	}
	inRead.Close()
	outWrite.Close()
	p := &Process{cmd: cmd, input: inWrite, output: outRead, session: session, options: options, ready: make(chan struct{}), stop: make(chan struct{}), done: make(chan struct{}), readDone: make(chan struct{})}
	p.unsubscribe = session.OnRevoke(p.signalStop)
	go p.serve()
	go p.watch(ctx)
	go p.wait()
	go func() {
		p.exchange.Lock()
		err := writeFrame(p.input, Init{Kind: "init", Version: 1, Entry: session.EntryURL(), Panel: options.Panel, Title: session.Binding().Title, CSP: uiview.CSP, ProbeTarget: options.ProbeTarget})
		p.exchange.Unlock()
		if err != nil {
			p.fail("UI 初始化写入失败")
		}
	}()
	timer := time.NewTimer(options.StartupTimeout)
	defer timer.Stop()
	select {
	case <-p.ready:
		failed = false
		return p, nil
	case <-p.done:
	case <-ctx.Done():
	case <-timer.C:
	}
	// An immediately closing guest may have completed before Start was scheduled.
	select {
	case <-p.ready:
		failed = false
		return p, nil
	default:
	}
	p.fail("UI 客体启动或握手失败")
	p.kill()
	<-p.done
	return nil, errors.New("UI 客体启动或握手失败")
}
func (p *Process) PID() int { return p.cmd.Process.Pid }
func (p *Process) Wait() Result {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return Result{Report: append(json.RawMessage(nil), p.result.Report...), Err: p.result.Err}
}
func (p *Process) signalStop() { p.stopOnce.Do(func() { close(p.stop) }) }
func (p *Process) Stop()       { p.session.Revoke(); p.signalStop(); <-p.done }
func (p *Process) fail(message string) {
	p.mu.Lock()
	if p.result.Err == nil {
		p.result.Err = errors.New(message)
	}
	p.mu.Unlock()
	p.session.Revoke()
	p.signalStop()
}
func (p *Process) kill() { _ = p.cmd.Process.Kill(); _ = p.input.Close(); _ = p.output.Close() }
func (p *Process) watch(ctx context.Context) {
	lifetime := time.NewTimer(p.options.Lifetime)
	defer lifetime.Stop()
	select {
	case <-p.done:
		return
	case <-p.stop:
	case <-ctx.Done():
		p.fail("UI 客体上下文已取消")
	case <-lifetime.C:
		p.fail("UI 客体运行超时")
	}
	grace := time.NewTimer(p.options.ShutdownGrace)
	defer grace.Stop()
	go func() {
		p.exchange.Lock()
		defer p.exchange.Unlock()
		_ = writeFrame(p.input, response{Kind: "revoke"})
	}()
	select {
	case <-p.done:
		return
	case <-grace.C:
		p.mu.Lock()
		if !p.finished && p.result.Err == nil {
			p.result.Err = errors.New("UI 客体未响应撤销")
		}
		p.mu.Unlock()
		p.kill()
	}
}
func (p *Process) wait() {
	err := p.cmd.Wait()
	_ = p.input.Close()
	_ = p.output.Close()
	<-p.readDone
	p.mu.Lock()
	if p.result.Err == nil && (err != nil || !p.finished) {
		p.result.Err = errors.New("UI 客体异常退出或未完成关闭")
	}
	p.mu.Unlock()
	p.session.Revoke()
	p.unsubscribe()
	close(p.done)
}
func (p *Process) serve() {
	defer close(p.readDone)
	scanner := newScanner(p.output)
	var sequence uint64
	hello := false
	served := 0
	requests := 0
	for scanner.Scan() {
		var req request
		if decode(scanner.Bytes(), &req) != nil || (req.Kind != "focused" && req.Control != 0) || (req.Kind != "window-action" && req.WindowAction != "") || req.ID != sequence+1 || req.ID == 0 {
			p.fail("UI 管道协议无效")
			return
		}
		if req.Kind != "focused" {
			requests++
			if requests > maxRequests {
				p.fail("UI 请求预算耗尽")
				return
			}
		}
		sequence = req.ID
		p.mu.Lock()
		finished := p.finished
		p.mu.Unlock()
		if finished {
			p.fail("UI 客体完成后继续发送请求")
			return
		}
		p.exchange.Lock()
		res := response{Kind: "reply", ID: req.ID}
		invalid := false
		var focusAck chan bool
		switch req.Kind {
		case "hello":
			if hello || req.ID != 1 || req.Version != 1 || req.URL != "" || len(req.Message) > 0 || len(req.Report) > 0 {
				invalid = true
				break
			}
			hello = true
			res.OK = true
		case "focused":
			if !hello || req.Control == 0 || req.Version != 0 || req.URL != "" || len(req.Message) > 0 || len(req.Report) > 0 {
				invalid = true
				break
			}
			p.mu.Lock()
			focusAck = p.focusPending[req.Control]
			delete(p.focusPending, req.Control)
			p.mu.Unlock()
			if focusAck == nil {
				invalid = true
				break
			}
			res.OK = p.session.Active()
		case "window-action":
			if !hello || req.Version != 0 || req.URL != "" || len(req.Message) > 0 || len(req.Report) > 0 || !p.options.Panel || p.options.WindowAction == nil || (req.WindowAction != "detach" && req.WindowAction != "back") {
				invalid = true
				break
			}
			res.OK = p.session.Active() && p.options.WindowAction(req.WindowAction)
			if res.OK && req.WindowAction == "back" {
				p.session.Revoke()
			}
		case "resource":
			if !hello || req.Version != 0 || len(req.URL) > 1024 || len(req.Message) > 0 || len(req.Report) > 0 {
				invalid = true
				break
			}
			resource, ok := p.session.Resource(req.URL)
			if ok {
				served += len(resource.Data)
				if served > maxResourceBytes {
					invalid = true
					break
				}
				res.OK = true
				res.Resource = &resource
			}
		case "message":
			if !hello || req.Version != 0 || req.URL != "" || len(req.Message) > uiview.MaxMessageBytes || len(req.Report) > 0 {
				invalid = true
				break
			}
			action, err := p.session.Message(req.Message)
			res.OK = err == nil
			res.Action = action
			if action == uiview.Ready {
				p.readyOnce.Do(func() { close(p.ready) })
			}
		case "revoke":
			if !hello || req.Version != 0 || req.URL != "" || len(req.Message) > 0 || len(req.Report) > 0 {
				invalid = true
				break
			}
			p.session.Revoke()
			res.OK = true
		case "probe-disable":
			if !hello || p.options.ProbeDisable == nil || req.Version != 0 || req.URL != "" || len(req.Message) > 0 || len(req.Report) > 0 {
				invalid = true
				break
			}
			res.OK = p.session.Active() && p.options.ProbeDisable() == nil && !p.session.Active()
		case "finish":
			if !hello || req.Version != 0 || req.URL != "" || len(req.Message) > 0 || len(req.Report) > 4096 || !json.Valid(req.Report) {
				invalid = true
				break
			}
			p.session.Revoke()
			p.mu.Lock()
			p.finished = true
			p.result.Report = append(json.RawMessage(nil), req.Report...)
			p.mu.Unlock()
			res.OK = true
		default:
			invalid = true
		}
		if invalid {
			p.exchange.Unlock()
			p.fail("UI 管道协议或预算无效")
			return
		}
		err := writeFrame(p.input, res)
		p.exchange.Unlock()
		if err != nil {
			p.fail("UI 管道写入失败")
			return
		}
		if focusAck != nil {
			select {
			case focusAck <- res.OK:
			default:
			}
		}
	}
	p.mu.Lock()
	finished := p.finished
	p.mu.Unlock()
	if !finished {
		p.fail("UI 管道提前关闭或格式无效")
	}
}
