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
		// AddTask 将 task 加入容器中，当返回 true 时，调用 Execute
		AddTask(task any) bool
		// Execute 执行 tasks
		Execute(tasks any)
		// RemoveAll 移除容器中的 tasks，并返回它们
		RemoveAll() any
	}

	// PeriodicalExecutor 用于周期性执行任务
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
		// 存在的意义：确保在执行器退出时，确保所有任务都执行完成（TODO）
		inflight  int32                                     // 用来判断是否可以退出当前 backgroundFlush
		guarded   bool                                      // 为 false 时，允许启动 backgroundFlush
		newTicker func(duration time.Duration) timex.Ticker // 时间间隔器
		lock      sync.Mutex
	}
)

// NewPeriodicalExecutor 间隔时间执行一次刷新
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
		// vals 是从 RemoveAll 方法取出的 tasks，将全部 tasks 通过 commander 传递
		// 接收是在 backgroundFlush 方法中
		pe.commander <- vals
		// 阻塞等待放行
		<-pe.confirmChan
	}
}

// Flush 强制执行 task
func (pe *PeriodicalExecutor) Flush() bool {
	// 本质：wg.Add(1)
	pe.enterExecution()
	// 匿名函数，相当于将 RemoveAll 执行结果作为参数传递
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
		// 允许一个协程去启动 backgroundFlush
		if !pe.guarded {
			// 进入后置 true，防止其他协程再次启动
			pe.guarded = true
			// 这里使用 defer 是为了快速执行 Unlock
			// backgroundFlush 后台协程刷新 task
			defer pe.backgroundFlush()
		}
		pe.lock.Unlock()
	}()

	// 实际调用的是使用方实现的 AddTask 方法
	// 将 task 添加到容器中，如果返回 true，将全部 tasks 取出
	if pe.container.AddTask(task) {
		atomic.AddInt32(&pe.inflight, 1)
		// 实际调用的是使用方实现的 RemoveAll 方法
		// 移除并返回全部的 tasks
		return pe.container.RemoveAll(), true
	}

	return nil, false
}

// 后台执行 task，同一时间仅执行一个
func (pe *PeriodicalExecutor) backgroundFlush() {
	go func() {
		// 返回前再次刷新，防止丢失 task
		defer pe.Flush()

		// 创建一个时间间隔器，用于 interval 间隔执行一次
		ticker := pe.newTicker(pe.interval)
		defer ticker.Stop()

		// 用途：当同时满足两个 select 分支时，可能存在这样的场景：
		// 1. select case 执行 commander 获取到 tasks 后，置为 true
		// 2. 下次执行定时器的 case，跳过定时器中的执行
		// 疑问：为什么只跳过定时器的 case，而没有跳过 commander 的 case
		// 猜测：commander 的 case 中肯定是全部的 tasks，而定时器中的 case 则不一定，为了积攒更多的 tasks 一次执行，所以选择跳过
		var commanded bool
		// 记录最近执行时间，当 10 次间隔时间都没有新 task 产生，考虑退出该 backgroundFlush
		last := timex.Now()
		for {
			select {
			// 当 Add 返回 true，获取到全部 tasks，传入该 channel
			case vals := <-pe.commander: // 从 channel 拿到 []task
				commanded = true
				atomic.AddInt32(&pe.inflight, -1)
				// 本质：执行 wg.Add(1)
				pe.enterExecution()
				// 放开 Add 的阻塞，使得 Add 在 task 执行时不会被阻塞
				pe.confirmChan <- lang.Placeholder
				// 开始真正执行 task
				pe.executeTasks(vals)
				last = timex.Now()
			case <-ticker.Chan(): // interval 间隔执行一次
				// 置反跳过本次执行
				if commanded {
					commanded = false
				} else if pe.Flush() { // 强制执行 task
					last = timex.Now()
				} else if pe.shallQuit(last) { // 定时器本轮中没有新 task，会执行到该分支
					return
				}
			}
		}
	}()
}

// wg.Done() 的场景：
// 1. 执行完成 Execute 方法后执行
func (pe *PeriodicalExecutor) doneExecution() {
	pe.waitGroup.Done()
}

// wg.Add(1) 的场景：
// 1. 刷新时存在任务，需要执行
// 2. task 执行 AddTask 时返回了 true，将全部任务通过 channel 传递给 backgroundFlush 执行前
func (pe *PeriodicalExecutor) enterExecution() {
	pe.wgBarrier.Guard(func() {
		pe.waitGroup.Add(1)
	})
}

func (pe *PeriodicalExecutor) executeTasks(tasks any) bool {
	// 本质：wg.Done()
	defer pe.doneExecution()

	// 判断是否有 task
	ok := pe.hasTasks(tasks)
	// 只有有 task，就执行 Execute 方法
	if ok {
		// 同步调用
		threading.RunSafe(func() {
			// 实际调用的是使用方实现的 Execute 方法
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
	// 如果10次间隔时间，都没有 task，该考虑退出 backgroundFlush
	if timex.Since(last) <= pe.interval*idleRound {
		return
	}

	// checking pe.inflight and setting pe.guarded should be locked together
	// TODO
	pe.lock.Lock()
	if atomic.LoadInt32(&pe.inflight) == 0 {
		// 只有这里置为 false，才会开启新的 pe.backgroundFlush
		pe.guarded = false
		stop = true
	}
	pe.lock.Unlock()

	return
}
