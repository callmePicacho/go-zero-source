package v2

import (
	"github.com/zeromicro/go-zero/core/syncx"
	"reflect"
	"sync"
	"time"
)

/*
在 v1 的基础上增加 Wait，确保 backgroundFlush 中执行完成才退出
需要使用 waitGroup，确保 Execute 执行完成才退出
本版本的问题：backgroundFlush 不会退出，如果整个PeriodicalExecutor，会导致 backgroundFlush 泄露
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
		commander   chan any       // 用于传递 tasks 的 channel
		interval    time.Duration  // 周期性间隔
		container   TaskContainer  // 执行器的容器
		waitGroup   sync.WaitGroup // 用于等待任务执行完成
		confirmChan chan struct{}  // 保证 wg.Wait 在 wg.Add(1) 之前执行
		wgBarrier   syncx.Barrier  // 避免 waitGroup 的竞态
		lock        sync.Mutex
	}
)

func NewPeriodicalExecutor(interval time.Duration, container TaskContainer) *PeriodicalExecutor {
	executor := &PeriodicalExecutor{
		commander:   make(chan any, 1),
		interval:    interval,
		container:   container,
		confirmChan: make(chan struct{}),
	}

	// 启动后台线程
	executor.backgroundFlush()

	return executor
}

// Add 任务添加，当AddTask返回为true，执行Execute
func (pe *PeriodicalExecutor) Add(task any) {
	// 返回 true，取出全部 tasks
	if vals, ok := pe.addAndCheck(task); ok {
		// 传入 vals，执行 Execute
		pe.commander <- vals
		// 阻塞Add，保证 wg.Add(1) 执行完成
		<-pe.confirmChan
	}
}

// AddTask 和 RemoveAll 加锁，但是写入 pe.commander 不需要加锁，
// 不然由于阻塞导致 Flush 获取不到锁，所以这里单独拆开
func (pe *PeriodicalExecutor) addAndCheck(task any) (any, bool) {
	pe.lock.Lock()
	defer pe.lock.Unlock()

	if ok := pe.container.AddTask(task); ok {
		return pe.container.RemoveAll(), true
	}

	return nil, false
}

// Flush 取出 container 中的全部 tasks，执行
func (pe *PeriodicalExecutor) Flush() {
	pe.lock.Lock()

	// 获取当前 container 中的全部 tasks
	vals := pe.container.RemoveAll()
	pe.lock.Unlock()

	// wg.Add(1)
	pe.enterExecution()

	// 执行
	pe.executeTasks(vals)
}

func (pe *PeriodicalExecutor) Wait() {
	pe.Flush()
	// 等待全部任务完成
	pe.wgBarrier.Guard(func() {
		pe.waitGroup.Wait()
	})
}

// 本质：wg.Add(1)，Execute 前执行
func (pe *PeriodicalExecutor) enterExecution() {
	// waitGroup.Add 本身线程安全
	pe.wgBarrier.Guard(func() {
		pe.waitGroup.Add(1)
	})
}

// 本质：wg.Done()，Execute 后执行
func (pe *PeriodicalExecutor) doneExecution() {
	pe.waitGroup.Done()
}

// wg.Add(1) 不在这里面执行，因为可能导致 wg.Wait 在 wg.Add(1) 前执行，例如：
// vals := <-pe.commander 收到tasks，但是还未执行executeTasks时
func (pe *PeriodicalExecutor) executeTasks(tasks any) {
	// wg.Done
	defer pe.doneExecution()

	ok := pe.hasTasks(tasks)
	if ok {
		pe.container.Execute(tasks)
	}
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
		return true
	}
}

func (pe *PeriodicalExecutor) backgroundFlush() {
	go func() {
		ticker := time.NewTicker(pe.interval)
		defer ticker.Stop()

		for {
			select {
			case vals := <-pe.commander: // 从 channel 拿到 tasks
				// wg.Add(1)
				pe.enterExecution()
				// 放开Add阻塞
				pe.confirmChan <- struct{}{}
				// 执行 Execute
				pe.executeTasks(vals)
			case <-ticker.C:
				pe.Flush()
			}
		}
	}()
}
