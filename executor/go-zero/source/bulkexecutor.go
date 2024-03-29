package executors

import "time"

const defaultBulkTasks = 1000

type (
	// BulkOption defines the method to customize a BulkExecutor.
	BulkOption func(options *bulkOptions)

	// BulkExecutor 是一个执行器，可以按照以下条件执行任务：
	// 1. 达到给定大小的任务数
	// 2. 经过间隔的刷新时间
	BulkExecutor struct {
		executor  *PeriodicalExecutor
		container *bulkContainer
	}

	bulkOptions struct {
		cachedTasks   int           // 最大任务数
		flushInterval time.Duration // 间隔刷新时间
	}
)

// NewBulkExecutor returns a BulkExecutor.
func NewBulkExecutor(execute Execute, opts ...BulkOption) *BulkExecutor {
	// 创建默认值
	options := newBulkOptions()
	for _, opt := range opts {
		opt(&options)
	}

	container := &bulkContainer{
		execute:  execute,
		maxTasks: options.cachedTasks,
	}

	executor := &BulkExecutor{
		executor:  NewPeriodicalExecutor(options.flushInterval, container), // 使用底层定时刷新的 executor
		container: container,                                               // 使用达到最大任务数提交的 container
	}

	return executor
}

// Add adds task into be.
func (be *BulkExecutor) Add(task any) error {
	be.executor.Add(task)
	return nil
}

// Flush forces be to flush and execute tasks.
func (be *BulkExecutor) Flush() {
	be.executor.Flush()
}

// Wait waits be to done with the task execution.
func (be *BulkExecutor) Wait() {
	be.executor.Wait()
}

// WithBulkTasks customizes a BulkExecutor with given tasks limit.
func WithBulkTasks(tasks int) BulkOption {
	return func(options *bulkOptions) {
		options.cachedTasks = tasks
	}
}

// WithBulkInterval customizes a BulkExecutor with given flush interval.
func WithBulkInterval(duration time.Duration) BulkOption {
	return func(options *bulkOptions) {
		options.flushInterval = duration
	}
}

// TaskContainer 接口的实现类
func newBulkOptions() bulkOptions {
	return bulkOptions{
		cachedTasks:   defaultBulkTasks,     // 默认任务数：1000
		flushInterval: defaultFlushInterval, // 默认刷新间隔：1s
	}
}

// 达到最大任务数提交的 container
type bulkContainer struct {
	tasks    []any
	execute  Execute
	maxTasks int
}

// AddTask 将任务添加到 tasks 中，达到最大任务数时，返回 true
func (bc *bulkContainer) AddTask(task any) bool {
	bc.tasks = append(bc.tasks, task)
	return len(bc.tasks) >= bc.maxTasks
}

// Execute 执行任务
func (bc *bulkContainer) Execute(tasks any) {
	vals := tasks.([]any)
	bc.execute(vals)
}

// RemoveAll 移除 tasks，并返回 tasks
func (bc *bulkContainer) RemoveAll() any {
	tasks := bc.tasks
	bc.tasks = nil
	return tasks
}
