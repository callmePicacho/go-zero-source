package rollingwindow

import (
	"sync"
	"time"
)

const (
	defaultSize     = 10
	defaultInterval = 100 * time.Millisecond
)

// Window 滑动窗口
type Window struct {
	buckets  []*bucket     // 桶
	size     int           // 桶的数量
	interval time.Duration // 统计间隔
	lock     sync.RWMutex
	lastTime time.Time // 上次更新时间
	current  int       // 当前所处桶
}

type WindowOption func(opt *Window)

func WithSize(size int) WindowOption {
	return func(opt *Window) {
		opt.size = size
	}
}

func WithInterval(interval time.Duration) WindowOption {
	return func(opt *Window) {
		opt.interval = interval
	}
}

func NewWindow(opts ...WindowOption) *Window {
	window := &Window{
		size:     defaultSize,
		interval: defaultInterval,
		lastTime: time.Now(),
		current:  0,
	}

	for _, opt := range opts {
		opt(window)
	}

	// 初始化桶
	window.buckets = make([]*bucket, window.size)
	for i := 0; i < window.size; i++ {
		window.buckets[i] = newBucket()
	}

	return window
}

// Add 添加统计值
// 核心要点：
// 1. 通过记录上一次更新时间，计算当前和上一次的间隔时间，进而根据桶的刻度算出过期桶的个数
// 2. 将过期桶中的统计值清除，同时在当前桶中添加统计值
func (w *Window) Add(val int64) {
	w.lock.Lock()
	defer w.lock.Unlock()

	// 计算当前所处桶位置
	w.current = w.currentBucket()

	// 添加统计值
	w.buckets[w.current].add(val)
}

// Reduce 获取统计值
// 核心要点：
// 1. 通过记录上一次更新时间，计算当前和上一次的间隔时间，进而根据桶的刻度算出过期桶的个数
// 2. 仅计算非过期桶的统计值，并返回
func (w *Window) Reduce() int64 {
	w.lock.RLock()
	defer w.lock.RUnlock()

	// 计算偏移量，偏移过的桶都是已过期的
	offset := w.span()
	// 全部桶都过期了，直接返回 0
	if offset == w.size {
		return 0
	}

	// 计算剩余桶的总统计值
	var sum int64 = 0

	// 需要计算的桶总数：桶的总数 - 过期桶数量
	total := w.size - offset
	// 未过期的桶的起始位置
	start := (w.current + offset + 1) % w.size
	for i := 0; i < total; i++ {
		idx := (start + i) % w.size
		sum += w.buckets[idx].val
	}

	return sum
}

// 计算当前所处桶位置
func (w *Window) currentBucket() int {
	// 计算偏移量
	offset := w.span()
	// 没动，直接返回当前桶位置
	if offset <= 0 {
		return w.current
	}

	// 偏移量，偏移经过的位置都是已过期的桶，需要将 (w.current, w.current+offset] 置为 0

	// 将已过期的桶置为0
	old := w.current + 1
	for i := 0; i < offset; i++ {
		// 取余为了循环
		idx := (old + i) % w.size
		w.buckets[idx].reset()
	}

	// 计算新的当前桶位置
	w.current = (w.current + offset) % w.size

	// 更新上次更新时间
	w.lastTime = time.Now()

	return w.current
}

// 计算偏移量
func (w *Window) span() int {
	offset := int(time.Since(w.lastTime) / w.interval)
	if 0 <= offset && offset <= w.size {
		return offset
	}

	// 最多计算一圈
	return w.size
}

// bucket 具体桶
type bucket struct {
	val int64 // 统计数据
}

func newBucket() *bucket {
	return new(bucket)
}

// reset 重置当前桶
func (b *bucket) reset() {
	b.val = 0
}

func (b *bucket) add(val int64) {
	b.val += val
}
