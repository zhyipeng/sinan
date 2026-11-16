// Package transport implements bounded newline-delimited JSON-RPC over stdio.
package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	protocol "sinan/packages/plugin-protocol"
)

type HostHandler func(context.Context, string, json.RawMessage) (any, error)

type Process struct {
	host        HostHandler
	hostIDs     map[uint64]bool
	hostContext context.Context
	hostCancel  context.CancelFunc
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	writeMu     sync.Mutex
	mu          sync.Mutex
	pending     map[uint64]chan protocol.Response
	next        atomic.Uint64
	done        chan struct{}
	exited      chan struct{}
	once        sync.Once
}

func Start(path, dir string) (*Process, error) {
	return StartWithHost(path, dir, nil)
}

func StartWithHost(path, dir string, host HostHandler) (*Process, error) {
	cmd := exec.Command(path)
	cmd.Dir = dir
	configureProcess(cmd)
	// Forward only named desktop/session variables, never the entire host
	// environment or arbitrary plugin stderr.
	cmd.Env = processEnvironment(runtime.GOOS, os.Getenv)
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
	p.host = host
	p.hostIDs = make(map[uint64]bool)
	p.hostContext, p.hostCancel = context.WithCancel(context.Background())
	go p.read(out)
	go func() { _ = cmd.Wait(); p.finish(); close(p.exited) }()
	return p, nil
}

func (p *Process) read(out io.Reader) {
	defer func() { p.finish(); _ = p.cmd.Process.Kill() }()
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), protocol.MaxClipboardMessageBytes)
	for scanner.Scan() {
		var envelope struct {
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &envelope) != nil {
			break
		}
		if envelope.Method != "" {
			var req protocol.Request
			if json.Unmarshal(scanner.Bytes(), &req) != nil || req.JSONRPC != "2.0" || req.ID == 0 {
				break
			}
			if len(scanner.Bytes()) >= protocol.MaxMessageBytes && req.Method != "clipboard.write" {
				break
			}
			p.mu.Lock()
			invalid := p.hostIDs[req.ID] || len(p.hostIDs) >= 4
			if !invalid {
				p.hostIDs[req.ID] = true
			}
			p.mu.Unlock()
			if invalid {
				return
			}
			go p.serveHost(req)
			continue
		}
		if len(scanner.Bytes()) >= protocol.MaxMessageBytes {
			break
		}
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
}

func (p *Process) finish() { p.once.Do(func() { p.hostCancel(); close(p.done); _ = p.stdin.Close() }) }

func (p *Process) serveHost(req protocol.Request) {
	defer func() { p.mu.Lock(); delete(p.hostIDs, req.ID); p.mu.Unlock() }()
	defer func() {
		if recover() != nil {
			p.finish()
			_ = p.cmd.Process.Kill()
		}
	}()
	ctx, cancel := context.WithTimeout(p.hostContext, 2*time.Second)
	defer cancel()
	response := protocol.Response{JSONRPC: "2.0", ID: req.ID}
	var value any
	var err error
	if p.host == nil {
		err = errors.New("宿主能力不可用")
	} else {
		value, err = p.host(ctx, req.Method, req.Params)
	}
	if err != nil {
		response.Error = &protocol.Error{Code: -32000, Message: err.Error()}
	} else {
		response.Result, err = json.Marshal(value)
	}
	if err != nil && response.Error == nil {
		response.Error = &protocol.Error{Code: -32000, Message: "宿主响应无效"}
	}
	message, _ := json.Marshal(response)
	if len(message) >= protocol.MaxClipboardMessageBytes {
		p.finish()
		return
	}
	// Stop a peer that no longer consumes its pipe, including a blocked writer.
	written := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			select {
			case <-written:
				return
			default:
				p.finish()
			}
		case <-written:
		}
	}()
	p.writeMu.Lock()
	_, _ = p.stdin.Write(append(message, '\n'))
	p.writeMu.Unlock()
	close(written)
}

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
	limit := protocol.MaxMessageBytes
	if method == "command.executeImage" {
		limit = protocol.MaxImageCommandBytes
	}
	if err != nil || len(message)+1 >= limit {
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
