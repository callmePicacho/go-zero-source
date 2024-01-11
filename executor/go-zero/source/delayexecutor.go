package executors

import (
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/threading"
)

// A DelayExecutor delays a tasks on given delay interval.
type DelayExecutor struct {
	fn        func()
	delay     time.Duration
	triggered bool
	lock      sync.Mutex
}

// NewDelayExecutor returns a DelayExecutor with given fn and delay.
func NewDelayExecutor(fn func(), delay time.Duration) *DelayExecutor {
	return &DelayExecutor{
		fn:    fn,
		delay: delay,
	}
}

// Trigger 在给定的延迟之后触发任务，可以多次触发
func (de *DelayExecutor) Trigger() {
	de.lock.Lock()
	defer de.lock.Unlock()

	// 确保只执行一次
	if de.triggered {
		return
	}

	de.triggered = true
	threading.GoSafe(func() {
		timer := time.NewTimer(de.delay)
		defer timer.Stop()
		<-timer.C

		// 在执行之前将 triggered 置为 false，确保在 fn 执行时依然能被触发
		de.lock.Lock()
		de.triggered = false
		de.lock.Unlock()
		de.fn()
	})
}
