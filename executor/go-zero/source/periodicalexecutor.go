package executors

import (
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zeromicro/go-zero/core/lang"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/syncx"
	"github.com/zeromicro/go-zero/core/threading"
	"github.com/zeromicro/go-zero/core/timex"
)

const idleRound = 10

type (
	// TaskContainer 接口定义了一个可以作为执行器的底层容器，用于周期性执行任务。
	TaskContainer interface {
		// AddTask 将 task 加入容器中
		AddTask(task any) bool
		// Execute 执行 tasks
		Execute(tasks any)
		// RemoveAll 移除容器中的 tasks，并返回它们
		RemoveAll() any
	}

	// PeriodicalExecutor 用于周期性执行任务。
	PeriodicalExecutor struct {
		commander   chan any                  // 用于传递 tasks 的 chan
		interval    time.Duration             // 周期性间隔
		container   TaskContainer             // 执行器的容器
		waitGroup   sync.WaitGroup            // 用于等待任务执行完成
		wgBarrier   syncx.Barrier             // 避免 waitGroup 的竞态
		confirmChan chan lang.PlaceholderType // 阻塞 Add()，避免 wg.Wait() 在 wg.Add(1) 前进行
		// +1 时机：当 Add task 返回 true 时，+1后执行RemoveAll()，获取 tasks 写入 channel
		// -1 时机：后台 goroutine 通过 channel 获取到 tasks 时
		// 判等0时机：当多轮时间间隔都无 task 时，决定是否需要退出 backgroundFlush
		inflight  int32                                     // 用来判断是否可以退出当前 backgroundFlush
		guarded   bool                                      // 为 false 时，允许启动 backgroundFlush
		newTicker func(duration time.Duration) timex.Ticker // 时间间隔器
		lock      sync.Mutex
	}
)

// NewPeriodicalExecutor returns a PeriodicalExecutor with given interval and container.
func NewPeriodicalExecutor(interval time.Duration, container TaskContainer) *PeriodicalExecutor {
	executor := &PeriodicalExecutor{
		// buffer 1 to let the caller go quickly
		commander:   make(chan any, 1),
		interval:    interval,
		container:   container,
		confirmChan: make(chan lang.PlaceholderType),
		newTicker: func(d time.Duration) timex.Ticker {
			return timex.NewTicker(d)
		},
	}
	// 优雅退出
	proc.AddShutdownListener(func() {
		executor.Flush()
	})

	return executor
}

// Add 加入 task
func (pe *PeriodicalExecutor) Add(task any) {
	if vals, ok := pe.addAndCheck(task); ok {
		// vals 是全部的 task 列表
		pe.commander <- vals
		// 阻塞等待放行
		<-pe.confirmChan
	}
}

// Flush 强制执行 task
func (pe *PeriodicalExecutor) Flush() bool {
	// 本质：wg.Add(1)
	pe.enterExecution()
	return pe.executeTasks(func() any {
		pe.lock.Lock()
		defer pe.lock.Unlock()
		// 移除并返回全部 task
		return pe.container.RemoveAll()
	}())
}

// Sync 线程安全的执行 fn
func (pe *PeriodicalExecutor) Sync(fn func()) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	fn()
}

// Wait 等待 task 执行完成
func (pe *PeriodicalExecutor) Wait() {
	pe.Flush()
	pe.wgBarrier.Guard(func() {
		pe.waitGroup.Wait()
	})
}

func (pe *PeriodicalExecutor) addAndCheck(task any) (any, bool) {
	pe.lock.Lock()
	defer func() {
		if !pe.guarded {
			pe.guarded = true
			// defer to unlock quickly
			defer pe.backgroundFlush()
		}
		pe.lock.Unlock()
	}()

	// 将 task 添加到容器中，根据返回值判断是否需要执行 task
	if pe.container.AddTask(task) {
		atomic.AddInt32(&pe.inflight, 1)
		// 移除并返回全部的 task
		return pe.container.RemoveAll(), true
	}

	return nil, false
}

func (pe *PeriodicalExecutor) backgroundFlush() {
	go func() {
		// 返回前再次刷新，防止丢失 task
		defer pe.Flush()

		ticker := pe.newTicker(pe.interval)
		defer ticker.Stop()

		var commanded bool
		last := timex.Now()
		for {
			select {
			case vals := <-pe.commander: // 从 channel 拿到 []task
				commanded = true
				atomic.AddInt32(&pe.inflight, -1)
				// 本质：执行 wg.Add(1)
				pe.enterExecution()
				// 放开 Add 的阻塞
				pe.confirmChan <- lang.Placeholder
				// 开始真正执行 task
				pe.executeTasks(vals)
				last = timex.Now()
			case <-ticker.Chan(): // interval 间隔执行一次
				if commanded {
					commanded = false
				} else if pe.Flush() { // 强制执行 task
					last = timex.Now()
				} else if pe.shallQuit(last) {
					return
				}
			}
		}
	}()
}

func (pe *PeriodicalExecutor) doneExecution() {
	pe.waitGroup.Done()
}

func (pe *PeriodicalExecutor) enterExecution() {
	pe.wgBarrier.Guard(func() {
		pe.waitGroup.Add(1)
	})
}

func (pe *PeriodicalExecutor) executeTasks(tasks any) bool {
	// 本质：wg.Done()
	defer pe.doneExecution()

	// 简单判断使用有 task
	ok := pe.hasTasks(tasks)
	if ok {
		threading.RunSafe(func() {
			// 执行接口定义的 Execute 方法
			pe.container.Execute(tasks)
		})
	}

	return ok
}

func (pe *PeriodicalExecutor) hasTasks(tasks any) bool {
	if tasks == nil {
		return false
	}

	val := reflect.ValueOf(tasks)
	switch val.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice:
		return val.Len() > 0
	default:
		// unknown type, let caller execute it
		return true
	}
}

func (pe *PeriodicalExecutor) shallQuit(last time.Duration) (stop bool) {
	if timex.Since(last) <= pe.interval*idleRound {
		return
	}

	// checking pe.inflight and setting pe.guarded should be locked together
	pe.lock.Lock()
	if atomic.LoadInt32(&pe.inflight) == 0 {
		// 只有这里置为 false，才会开启新的 pe.backgroundFlush
		pe.guarded = false
		stop = true
	}
	pe.lock.Unlock()

	return
}
