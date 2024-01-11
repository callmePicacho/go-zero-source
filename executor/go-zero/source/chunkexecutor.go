package executors

import "time"

const defaultChunkSize = 1024 * 1024 // 1M

type (
	// ChunkOption defines the method to customize a ChunkExecutor.
	ChunkOption func(options *chunkOptions)

	// ChunkExecutor 是一个执行器，满足以下条件执行任务：
	// 1. 达到给定大小
	// 2. 经过间隔的刷新时间
	ChunkExecutor struct {
		executor  *PeriodicalExecutor
		container *chunkContainer
	}

	chunkOptions struct {
		chunkSize     int
		flushInterval time.Duration
	}
)

// NewChunkExecutor returns a ChunkExecutor.
func NewChunkExecutor(execute Execute, opts ...ChunkOption) *ChunkExecutor {
	// 默认配置项
	options := newChunkOptions()
	for _, opt := range opts {
		opt(&options)
	}

	container := &chunkContainer{
		execute:      execute,
		maxChunkSize: options.chunkSize,
	}
	executor := &ChunkExecutor{
		executor:  NewPeriodicalExecutor(options.flushInterval, container),
		container: container,
	}

	return executor
}

// Add adds task with given chunk size into ce.
func (ce *ChunkExecutor) Add(task any, size int) error {
	ce.executor.Add(chunk{
		val:  task,
		size: size,
	})
	return nil
}

// Flush forces ce to flush and execute tasks.
func (ce *ChunkExecutor) Flush() {
	ce.executor.Flush()
}

// Wait waits the execution to be done.
func (ce *ChunkExecutor) Wait() {
	ce.executor.Wait()
}

// WithChunkBytes customizes a ChunkExecutor with the given chunk size.
func WithChunkBytes(size int) ChunkOption {
	return func(options *chunkOptions) {
		options.chunkSize = size
	}
}

// WithFlushInterval customizes a ChunkExecutor with the given flush interval.
func WithFlushInterval(duration time.Duration) ChunkOption {
	return func(options *chunkOptions) {
		options.flushInterval = duration
	}
}

func newChunkOptions() chunkOptions {
	return chunkOptions{
		chunkSize:     defaultChunkSize,     // 默认 1M
		flushInterval: defaultFlushInterval, // 默认 1s
	}
}

// 达到最大字节数提交的 container
type chunkContainer struct {
	tasks        []any
	execute      Execute
	size         int
	maxChunkSize int
}

// AddTask 添加任务，就是 size，如果 size 大于 maxChunkSize，则返回 true
func (bc *chunkContainer) AddTask(task any) bool {
	ck := task.(chunk)
	bc.tasks = append(bc.tasks, ck.val)
	bc.size += ck.size
	return bc.size >= bc.maxChunkSize
}

// Execute 执行任务
func (bc *chunkContainer) Execute(tasks any) {
	vals := tasks.([]any)
	bc.execute(vals)
}

// RemoveAll 移除 tasks，并返回 tasks
func (bc *chunkContainer) RemoveAll() any {
	tasks := bc.tasks
	bc.tasks = nil
	bc.size = 0
	return tasks
}

type chunk struct {
	val  any
	size int
}
