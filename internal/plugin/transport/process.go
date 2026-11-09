// Package transport implements bounded newline-delimited JSON-RPC over stdio.
package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	protocol "sinan/packages/plugin-protocol"
)

type Process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan protocol.Response
	next    atomic.Uint64
	done    chan struct{}
	exited  chan struct{}
	once    sync.Once
}

func Start(path, dir string) (*Process, error) {
	cmd := exec.Command(path)
	cmd.Dir = dir
	// Do not forward the host's environment or log arbitrary plugin stderr.
	cmd.Env = []string{"LANG=en_US.UTF-8"}
	// nil routes stderr directly to the OS null device. io.Discard would
	// create a copy goroutine whose pipe can be held open by descendants.
	cmd.Stderr = nil
	cmd.WaitDelay = 500 * time.Millisecond
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.New("无法创建插件输入管道")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, errors.New("无法创建插件输出管道")
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return nil, errors.New("无法启动插件进程")
	}
	p := &Process{cmd: cmd, stdin: in, pending: make(map[uint64]chan protocol.Response), done: make(chan struct{}), exited: make(chan struct{})}
	go p.read(out)
	go func() { _ = cmd.Wait(); p.finish(); close(p.exited) }()
	return p, nil
}

func (p *Process) read(out io.Reader) {
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), protocol.MaxMessageBytes)
	for scanner.Scan() {
		var response protocol.Response
		if json.Unmarshal(scanner.Bytes(), &response) != nil || response.JSONRPC != "2.0" || response.ID == 0 || (response.Error == nil) == (response.Result == nil) {
			break
		}
		p.mu.Lock()
		ch := p.pending[response.ID]
		p.mu.Unlock()
		if ch != nil {
			select {
			case ch <- response:
			default:
			}
		}
	}
	p.finish()
	_ = p.cmd.Process.Kill()
}

func (p *Process) finish() { p.once.Do(func() { close(p.done); _ = p.stdin.Close() }) }

func (p *Process) Alive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *Process) Call(ctx context.Context, method string, params, result any) error {
	return p.call(ctx, method, params, result, false)
}

// CallSearch cancels one request without terminating cooperating plugins.
func (p *Process) CallSearch(ctx context.Context, params protocol.SearchParams, result *protocol.SearchResult) error {
	return p.call(ctx, "search.query", params, result, true)
}

func (p *Process) call(ctx context.Context, method string, params, result any, cooperative bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !p.Alive() {
		return errors.New("插件进程已退出")
	}
	id := p.next.Add(1)
	b, err := json.Marshal(params)
	if err != nil {
		return errors.New("请求参数无法编码")
	}
	message, err := json.Marshal(protocol.Request{JSONRPC: "2.0", ID: id, Method: method, Params: b})
	if err != nil || len(message)+1 >= protocol.MaxMessageBytes {
		return errors.New("请求超过消息大小限制")
	}
	ch := make(chan protocol.Response, 1)
	p.mu.Lock()
	if len(p.pending) >= 64 {
		p.mu.Unlock()
		return errors.New("插件请求数量超过限制")
	}
	p.pending[id] = ch
	p.mu.Unlock()
	writeFinished := make(chan struct{})
	cleanup := true
	defer func() {
		if cleanup {
			p.remove(id)
		}
	}()
	cancelRequest := func() {
		if cooperative {
			cleanup = false
			go p.cancelSearch(id, ch, writeFinished)
		} else {
			p.Stop()
		}
	}
	// A plugin may stop reading stdin; killing it on cancellation unblocks Write.
	writeDone := make(chan error, 1)
	go func() {
		defer close(writeFinished)
		p.writeMu.Lock()
		defer p.writeMu.Unlock()
		_, err := p.stdin.Write(append(message, '\n'))
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			return errors.New("插件通信写入失败")
		}
	case <-ctx.Done():
		cancelRequest()
		return ctx.Err()
	case <-p.done:
		return errors.New("插件进程已退出")
	}
	select {
	case response := <-ch:
		if response.Error != nil {
			return errors.New("插件返回执行错误")
		}
		if json.Unmarshal(response.Result, result) != nil {
			return errors.New("插件响应格式无效")
		}
		return nil
	case <-ctx.Done():
		cancelRequest()
		return ctx.Err()
	case <-p.done:
		return errors.New("插件进程已退出")
	}
}

// Stop forcibly terminates the v1 process and reaps it. Graceful shutdown is deferred.
func (p *Process) Stop() { p.finish(); _ = p.cmd.Process.Kill(); <-p.exited }

func (p *Process) remove(id uint64) { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }

// A response acknowledges cancellation. Kill only plugins that ignore the
// bounded grace period (including a pipe that stopped consuming messages).
func (p *Process) cancelSearch(id uint64, response <-chan protocol.Response, written <-chan struct{}) {
	defer p.remove(id)
	params, _ := json.Marshal(protocol.CancelParams{ID: id})
	message, _ := json.Marshal(protocol.Request{JSONRPC: "2.0", Method: "$/cancelRequest", Params: params})
	go func() {
		select {
		case <-written:
		case <-p.done:
			return
		}
		p.writeMu.Lock()
		defer p.writeMu.Unlock()
		if p.Alive() {
			_, _ = p.stdin.Write(append(message, '\n'))
		}
	}()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-response:
	case <-p.done:
	case <-timer.C:
		p.Stop()
	}
}
