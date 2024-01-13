package v1

import (
	"sync"
	"time"
)

/*
基础版本，只提供 Add 和 Flush 功能
缺陷：当 backgroundFlush 中拿到任务 Execute，可能业务主线程执行完成退出，需要添加 Wait 等待执行完成
*/

type (
	Execute func(tasks []any)

	TaskContainer interface {
		AddTask(task any) bool
		Execute(tasks any)
		RemoveAll() any
	}

	// PeriodicalExecutor 周期性执行器
	PeriodicalExecutor struct {
		commander chan any      // 用于传递 tasks 的 channel
		interval  time.Duration // 周期性间隔
		container TaskContainer // 执行器的容器
		lock      sync.Mutex
	}
)

func NewPeriodicalExecutor(interval time.Duration, container TaskContainer) *PeriodicalExecutor {
	executor := &PeriodicalExecutor{
		commander: make(chan any, 1),
		interval:  interval,
		container: container,
	}

	// 启动后台线程
	executor.backgroundFlush()

	return executor
}

// Add 任务添加，当AddTask返回为true，执行Execute
func (pe *PeriodicalExecutor) Add(task any) {
	pe.lock.Lock()
	defer pe.lock.Unlock()

	// 返回 true，取出全部 tasks
	if ok := pe.container.AddTask(task); ok {
		pe.commander <- pe.container.RemoveAll()
	}
}

// Flush 取出 container 中的全部 tasks，执行
func (pe *PeriodicalExecutor) Flush() {
	pe.lock.Lock()

	// 获取当前 container 中的全部 tasks
	vals := pe.container.RemoveAll()
	pe.lock.Unlock()

	// 执行
	pe.container.Execute(vals)
}

func (pe *PeriodicalExecutor) backgroundFlush() {
	go func() {
		ticker := time.NewTicker(pe.interval)
		defer ticker.Stop()

		for {
			select {
			case vals := <-pe.commander: // 从 channel 拿到 tasks
				// 执行
				pe.container.Execute(vals)
			case <-ticker.C:
				pe.Flush()
			}
		}
	}()
}
