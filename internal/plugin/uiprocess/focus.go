package uiprocess

import (
	"errors"
	"time"
)

// Focus is bounded and allows only one outstanding control. A late reply is
// drained without killing a healthy window or creating another focus request.
func (p *Process) Focus() error {
	if !p.session.Active() {
		return errors.New("视图已关闭，请重新打开")
	}
	p.mu.Lock()
	if len(p.focusPending) > 0 {
		p.mu.Unlock()
		return errors.New("窗口正在切换，请稍后重试")
	}
	if p.finished || p.focusSequence == ^uint64(0) {
		p.mu.Unlock()
		return errors.New("窗口聚焦不可用")
	}
	p.focusSequence++
	id := p.focusSequence
	ack := make(chan bool, 1)
	if p.focusPending == nil {
		p.focusPending = map[uint64]chan bool{}
	}
	p.focusPending[id] = ack
	p.mu.Unlock()
	go func() {
		p.exchange.Lock()
		var err error
		if !p.session.Active() {
			err = errors.New("revoked")
		} else {
			err = writeFrame(p.input, response{Kind: "focus", ID: id})
		}
		p.exchange.Unlock()
		if err != nil {
			p.mu.Lock()
			delete(p.focusPending, id)
			p.mu.Unlock()
			select {
			case ack <- false:
			default:
			}
		}
	}()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case ok := <-ack:
		if ok && p.session.Active() {
			return nil
		}
		return errors.New("视图已关闭，请重新打开")
	case <-p.session.Done():
		return errors.New("视图已关闭，请重新打开")
	case <-p.done:
		return errors.New("UI 客体已退出")
	case <-timer.C:
		return errors.New("窗口未及时响应聚焦，请稍后重试")
	}
}
