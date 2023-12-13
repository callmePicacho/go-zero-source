package collection

import (
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/timex"
)

type (
	// RollingWindowOption 传参函数
	RollingWindowOption func(rollingWindow *RollingWindow)

	// RollingWindow 滑动窗口结构体
	RollingWindow struct {
		lock     sync.RWMutex
		size     int           // 滑动窗口大小
		win      *window       // 窗口
		interval time.Duration // 滑动窗口刻度
		offset   int           // 当前窗口偏移量
		// 是否忽略当前窗口，默认不忽略
		// 当前正在写入的桶数据没有经过完整的窗口时间刻度，可能导致当前桶数据不准确
		ignoreCurrent bool
		// 最后写入桶的时间
		// 通过记录该值，每次写入时，能快速算出经过了多少偏移量
		// 而无需每次窗口刻度都进行计算
		lastTime time.Duration
	}
)

func NewRollingWindow(size int, interval time.Duration, opts ...RollingWindowOption) *RollingWindow {
	if size < 1 {
		panic("size must be greater than 0")
	}

	w := &RollingWindow{
		size:     size,
		win:      newWindow(size),
		interval: interval,
		lastTime: timex.Now(),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Add 将 v 添加到当前滑动窗口的桶中
func (rw *RollingWindow) Add(v float64) {
	rw.lock.Lock()
	defer rw.lock.Unlock()

	// 更新偏移量
	rw.updateOffset()
	// 向当前窗口的桶中添加数据
	rw.win.add(rw.offset, v)
}

// Reduce 在所有存储桶上运行fn，如果设置了ignoreCurrent，则忽略当前存储桶
func (rw *RollingWindow) Reduce(fn func(b *Bucket)) {
	rw.lock.RLock()
	defer rw.lock.RUnlock()

	var diff int
	// 计算偏移量，偏移过的桶都是已过期的
	span := rw.span()
	// diff 为需要计算的未过期的桶总数
	if span == 0 && rw.ignoreCurrent {
		// 特别的，当 ignoreCurrent 参数为 true 时，不统计当前桶，故 桶总数-1
		diff = rw.size - 1
	} else {
		// 当 ignoreCurrent 为 false，未过期的桶总数：桶总数 - 过期桶数量
		diff = rw.size - span
	}
	// 当需要统计的桶数据 > 0
	if diff > 0 {
		// 未过期的桶的起始位置，当前桶的下一个桶，是第一个过期的桶
		offset := (rw.offset + span + 1) % rw.size
		// 进行统计
		rw.win.reduce(offset, diff, fn)
	}
}

// 通过 lastTime 计算经过了多少偏移量
func (rw *RollingWindow) span() int {
	// 计算偏移量
	offset := int(timex.Since(rw.lastTime) / rw.interval)
	// 判断是否在窗口范围内，如果在窗口范围内直接返回
	if 0 <= offset && offset < rw.size {
		return offset
	}

	// 如果不在窗口范围内，则返回窗口大小
	// 相当于一个小优化，就算是 size 更大的窗口，也只会重置 size 个桶
	return rw.size
}

// 更新偏移量
func (rw *RollingWindow) updateOffset() {
	// 计算偏移量
	span := rw.span()
	// 如果偏移量小于等于0，说明还在当前桶，不进行后续操作
	if span <= 0 {
		return
	}

	offset := rw.offset
	// 重置过期的桶，清空 (offset, offset + span] 区间的桶数据
	for i := 0; i < span; i++ {
		// 取余是由于环形数组
		rw.win.resetBucket((offset + i + 1) % rw.size)
	}

	// 得到新的桶偏移量，新桶此时已经被前面重置过了
	rw.offset = (offset + span) % rw.size
	now := timex.Now()
	// 与时间刻度对齐，保证 lastTime 始终是 interval 的整数倍
	rw.lastTime = now - (now-rw.lastTime)%rw.interval
}

// Bucket 桶
type Bucket struct {
	// 桶内元素之和
	Sum float64
	// 桶的add次数
	Count int64
}

// 向桶中添加数据
func (b *Bucket) add(v float64) {
	// 累加
	b.Sum += v
	// 计数
	b.Count++
}

// 重置桶
func (b *Bucket) reset() {
	b.Sum = 0
	b.Count = 0
}

// 具体滑动窗口
type window struct {
	// 使用环形数组，一个桶标识一个时间刻度
	buckets []*Bucket
	// 窗口大小
	size int
}

// 初始化窗口
func newWindow(size int) *window {
	buckets := make([]*Bucket, size)
	for i := 0; i < size; i++ {
		buckets[i] = new(Bucket)
	}
	return &window{
		buckets: buckets,
		size:    size,
	}
}

// 向窗口中 offset 偏移量的桶添加数据 v
func (w *window) add(offset int, v float64) {
	w.buckets[offset%w.size].add(v)
}

// 从窗口中 start 偏移量开始，count 个桶执行 fn
func (w *window) reduce(start, count int, fn func(b *Bucket)) {
	for i := 0; i < count; i++ {
		fn(w.buckets[(start+i)%w.size])
	}
}

// 重置窗口中 offset 偏移量的桶
func (w *window) resetBucket(offset int) {
	w.buckets[offset%w.size].reset()
}

// IgnoreCurrentBucket 设置是否忽略当前窗口
func IgnoreCurrentBucket() RollingWindowOption {
	return func(w *RollingWindow) {
		w.ignoreCurrent = true
	}
}
