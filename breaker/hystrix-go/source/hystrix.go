package hystrix

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// 定义依赖于外部系统的方法，例如数据库请求
type runFunc func() error

// 定义服务中断期间执行的方法
type fallbackFunc func(error) error
type runFuncC func(context.Context) error
type fallbackFuncC func(context.Context, error) error

// CircuitError 自定义熔断器错误
type CircuitError struct {
	Message string
}

func (e CircuitError) Error() string {
	return "hystrix: " + e.Message
}

// command 代表了熔断器中单次执行的状态，通常用 "hystrix command" 来描述与熔断器配对的 run/fallback 函数。
type command struct {
	sync.Mutex

	ticket      *struct{}       // 代表从令牌桶中取出的一个令牌
	start       time.Time       // command 执行开始时间
	errChan     chan error      // command 执行错误
	finished    chan bool       // command 执行结束
	circuit     *CircuitBreaker // 熔断器对象
	run         runFuncC        // 应用程序
	fallback    fallbackFuncC   // 应用程序执行失败时的回调函数
	runDuration time.Duration   // 执行 run 函数的时长
	events      []string        // 存储事件类型信息
}

var (
	// ErrMaxConcurrency 达到最大并发量
	ErrMaxConcurrency = CircuitError{Message: "max concurrency"}
	// ErrCircuitOpen 熔断器打开状态
	ErrCircuitOpen = CircuitError{Message: "circuit open"}
	// ErrTimeout run 方法执行超时
	ErrTimeout = CircuitError{Message: "timeout"}
)

// Go 异步调用 run 方法，实际上是调用 GoC
func Go(name string, run runFunc, fallback fallbackFunc) chan error {
	runC := func(ctx context.Context) error {
		return run()
	}
	var fallbackC fallbackFuncC
	if fallback != nil {
		fallbackC = func(ctx context.Context, err error) error {
			return fallback(err)
		}
	}
	// 实际上是加上 context 调用 GoC
	return GoC(context.Background(), name, runC, fallbackC)
}

// GoC 异步调用 run 方法
func GoC(ctx context.Context, name string, run runFuncC, fallback fallbackFuncC) chan error {
	// 对于每个调用：实例化 command 对象
	cmd := &command{
		run:      run,
		fallback: fallback,
		start:    time.Now(),
		errChan:  make(chan error, 1),
		finished: make(chan bool, 1),
	}

	// dont have methods with explicit params and returns
	// let data come in and out naturally, like with any closure
	// explicit error return to give place for us to kill switch the operation (fallback)

	// 根据 name 获取熔断器对象
	circuit, _, err := GetCircuit(name)
	if err != nil {
		cmd.errChan <- err
		return cmd.errChan
	}
	cmd.circuit = circuit
	// 使用 cmd 内部的锁初始化 Cond
	ticketCond := sync.NewCond(cmd)
	ticketChecked := false
	// 将令牌还到令牌桶中
	returnTicket := func() {
		cmd.Lock()
		// 避免在令牌被取出之前释放令牌。
		for !ticketChecked {
			// 陷入阻塞
			ticketCond.Wait()
		}
		// 归还令牌
		cmd.circuit.executorPool.Return(cmd.ticket)
		cmd.Unlock()
	}
	returnOnce := &sync.Once{}
	// 上报状态事件
	reportAllEvent := func() {
		err := cmd.circuit.ReportEvent(cmd.events, cmd.start, cmd.runDuration)
		if err != nil {
			log.Printf(err.Error())
		}
	}

	// 执行 run
	go func() {
		// 并发控制
		defer func() { cmd.finished <- true }()

		// 根据熔断器状态判断不执行该请求
		if !cmd.circuit.AllowRequest() {
			cmd.Lock()
			// 此时根本没获取到令牌，为了保持一致性，依然选择归还令牌
			// 在归还令牌处兼容令牌为 nil 的场景
			ticketChecked = true
			// 通知释放令牌信息
			ticketCond.Signal()
			cmd.Unlock()
			returnOnce.Do(func() {
				// 归还令牌
				returnTicket()
				// 执行 fallback
				cmd.errorWithFallback(ctx, ErrCircuitOpen)
				// 上报状态事件
				reportAllEvent()
			})
			return
		}

		// As backends falter, requests take longer but don't always fail.
		//
		// When requests slow down but the incoming rate of requests stays the same, you have to
		// run more at a time to keep up. By controlling concurrency during these situations, you can
		// shed load which accumulates due to the increasing ratio of active commands to incoming requests.
		cmd.Lock()
		select {
		case cmd.ticket = <-circuit.executorPool.Tickets: // 能从令牌桶中成功获取到令牌，放行该请求
			ticketChecked = true
			ticketCond.Signal()
			cmd.Unlock()
		default: // 不能从令牌桶中获取到令牌，达到最大并发数量，执行 fallback
			ticketChecked = true
			ticketCond.Signal()
			cmd.Unlock()
			returnOnce.Do(func() {
				// 归还令牌
				returnTicket()
				// 达到最大并发数，执行 fallback
				cmd.errorWithFallback(ctx, ErrMaxConcurrency)
				reportAllEvent()
			})
			return
		}

		// 成功拿到令牌，执行 run
		runStart := time.Now()
		runErr := run(ctx)
		returnOnce.Do(func() {
			defer reportAllEvent()
			// 统计 run 执行时间
			cmd.runDuration = time.Since(runStart)
			// 归还令牌
			returnTicket()
			if runErr != nil {
				// 如果应用程序执行失败执行fallback函数
				cmd.errorWithFallback(ctx, runErr)
				return
			}
			// 追加 "success" 事件
			cmd.reportEvent("success")
		})
	}()

	go func() {
		// 启动定时器进行超时控制，默认 1 s
		timer := time.NewTimer(getSettings(name).Timeout)
		defer timer.Stop()

		select {
		// 前面执行 run 的协程成功执行完成（不论是否成功）
		case <-cmd.finished:
			// returnOnce has been executed in another goroutine
		case <-ctx.Done(): // context 取消
			returnOnce.Do(func() {
				returnTicket()
				cmd.errorWithFallback(ctx, ctx.Err())
				reportAllEvent()
			})
			return
		case <-timer.C: // cmd 超时
			returnOnce.Do(func() {
				returnTicket()
				cmd.errorWithFallback(ctx, ErrTimeout)
				reportAllEvent()
			})
			return
		}
	}()

	return cmd.errChan
}

// Do 同步调用 run 方法，实际上是调用 GoC
func Do(name string, run runFunc, fallback fallbackFunc) error {
	runC := func(ctx context.Context) error {
		return run()
	}
	var fallbackC fallbackFuncC
	if fallback != nil {
		fallbackC = func(ctx context.Context, err error) error {
			return fallback(err)
		}
	}
	// 实际上是加上 context 调用 DoC
	return DoC(context.Background(), name, runC, fallbackC)
}

// DoC 同步调用 run 方法，实际上是调用 GoC
func DoC(ctx context.Context, name string, run runFuncC, fallback fallbackFuncC) error {
	done := make(chan struct{}, 1)

	r := func(ctx context.Context) error {
		err := run(ctx)
		if err != nil {
			return err
		}

		// 异步转同步，通知上层
		done <- struct{}{}
		return nil
	}

	f := func(ctx context.Context, e error) error {
		err := fallback(ctx, e)
		if err != nil {
			return err
		}

		done <- struct{}{}
		return nil
	}

	// 调用 GoC
	var errChan chan error
	if fallback == nil {
		errChan = GoC(ctx, name, r, nil)
	} else {
		errChan = GoC(ctx, name, r, f)
	}

	// 等待执行完成或报错，再返回
	select {
	case <-done:
		return nil
	case err := <-errChan:
		return err
	}
}

// 线程安全地将 eventType 追加到 events 中
func (c *command) reportEvent(eventType string) {
	c.Lock()
	defer c.Unlock()

	c.events = append(c.events, eventType)
}

// errorWithFallback 根据错误类型触发 fallback，并记录相应的 metric 事件
func (c *command) errorWithFallback(ctx context.Context, err error) {
	eventType := "failure"
	if err == ErrCircuitOpen { // 熔断器打开状态
		eventType = "short-circuit"
	} else if err == ErrMaxConcurrency { // 最大并发数
		eventType = "rejected"
	} else if err == ErrTimeout { // 超时
		eventType = "timeout"
	} else if err == context.Canceled { // context 取消
		eventType = "context_canceled"
	} else if err == context.DeadlineExceeded { // context 超时
		eventType = "context_deadline_exceeded"
	}

	// 追加到 events 中
	c.reportEvent(eventType)
	// 尝试执行 fallback
	fallbackErr := c.tryFallback(ctx, err)
	if fallbackErr != nil {
		c.errChan <- fallbackErr // 将错误写回 errChan，返回给实际的 GoC 调用方
	}
}

// 尝试执行 fallback，如果成功执行 fallback，返回 nil
func (c *command) tryFallback(ctx context.Context, err error) error {
	if c.fallback == nil {
		// 如果没有定义 fallback，则直接返回错误
		return err
	}

	// 执行 fallback
	fallbackErr := c.fallback(ctx, err)
	if fallbackErr != nil {
		c.reportEvent("fallback-failure")
		return fmt.Errorf("fallback failed with '%v'. run error was '%v'", fallbackErr, err)
	}

	c.reportEvent("fallback-success")

	return nil
}
