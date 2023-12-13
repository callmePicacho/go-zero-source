## 为什么需要滑动窗口
我们日常开发中，可能会被问到这样一个问题，如何知道系统瞬时的 QPS 有多大？
对于这样的数据统计需求，如果直接以秒为单位建立 bucket，会导致统计结果误差巨大，例如：

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1700138963929-c38eae44-5da7-46d2-b11d-93f4a2cdcf16.png#averageHue=%23fcfcfc&clientId=u8771a01c-f219-4&from=paste&height=661&id=u0550ba43&originHeight=661&originWidth=874&originalType=binary&ratio=1&rotation=0&showTitle=false&size=26836&status=done&style=none&taskId=u2200925a-d64a-436f-ac9c-e817de63dc0&title=&width=874)

上面的例子中，可以看到相同的请求量，由于统计粒度太大，导致结果误差很大，且我们如果想要统计当前时间往前，一秒中的 QPS，该怎么做呢？
其实很简单，细化统计粒度，例如 1s 中使用 10000 个 bucket 存放统计结果，再算均值，误差绝对小很多，可以得出结论：粒度越细，误差越小
## 滑动窗口原理
滑动窗口又是什么呢？简单来说，就是划分足够粒度的 bucket，一次使用多个 bucket 统计的总和作为结果，替代上面大粒度的 bucket，并不断移动，始终保持 bucket 总数的数据结构。
还是上面 QPS 的例子

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1700139866841-ba94c34c-dfd5-4bcd-b110-0e3e0018c3a1.png#averageHue=%23f8f8f8&clientId=u8771a01c-f219-4&from=paste&height=292&id=u605c9ea0&originHeight=292&originWidth=784&originalType=binary&ratio=1&rotation=0&showTitle=false&size=18276&status=done&style=none&taskId=ub19f45e0-50f4-4df5-bd15-8346cbaa5ce&title=&width=784)

这样既能统计到瞬时 QPS，也能让整个 QPS 统计误差更小，更有助于上层应用做出判断
我们可以称多个 bucket 组成的统计长度为：窗口长度，例如 QPS 统计，窗口周期为1s
称 bucket 的精度为：统计周期，例如上文中 bucket 的窗口长度为1/10s
更直观说：滑动窗口就是使用多个 bucket 始终保持同样窗口长度，进行统计的一种数据结构
## 滑动窗口如何实现

1. 选用循环队列作为bucket，单个 bucket 刻度代表统计周期，bucket 数量*统计周期 = 窗口长度
2. 随着时间流逝，清空之前 bucket 中的统计数据，往新的 bucket 中填充统计数据

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1700142037873-4254a213-7978-4828-b9ad-151b7f105bce.png#averageHue=%23faf4f4&clientId=u8771a01c-f219-4&from=paste&height=279&id=u9f88f53c&originHeight=279&originWidth=592&originalType=binary&ratio=1&rotation=0&showTitle=false&size=6749&status=done&style=none&taskId=ub13158c0-bd76-4b38-b215-1a296efc6ff&title=&width=592)

3. 核心功能包括：
- add，增加统计值，通过记录上次更新时间和上次桶位置，结合当前调用时间，算出当前桶位置，删除过期桶数据，往当前桶中增加统计值，核心要点：
   - 通过记录上一次更新时间，计算当前和上一次的间隔时间，进而根据桶的刻度算出过期桶的个数
   - 将过期桶中的统计值清除，同时在当前桶中添加统计值
> 重点考虑哪些过期桶需要重置

- sum，取出窗口长度的统计值总和，通过记录上次更新时间和上次桶位置，结合当前调用时间，算出哪些桶已过期，只统计未过期的桶数据，核心要点：
   - 通过记录上一次更新时间，计算当前和上一次的间隔时间，进而根据桶的刻度算出过期桶的个数
   - 仅计算非过期桶的统计值
> 重点考虑需要统计哪些非过期桶

第一个过期的桶，永远是当前指向的桶的下一个
```go
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
	// 未过期的桶的起始位置，第一个未过期的桶应该是 w.current+offset+1
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

```
```go
package rollingwindow

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

const duration = time.Millisecond * 500

func TestRollingWindowAdd(t *testing.T) {
	r := NewWindow(WithSize(3), WithInterval(duration))
	assert.Equal(t, int64(0), r.Reduce())
	r.Add(1)
	assert.Equal(t, int64(1), r.Reduce())
	elapse()
	r.Add(2)
	r.Add(3)
	assert.Equal(t, int64(6), r.Reduce())
	elapse()
	r.Add(4)
	r.Add(5)
	r.Add(6)
	assert.Equal(t, int64(21), r.Reduce())
	elapse()
	r.Add(7)
	assert.Equal(t, int64(27), r.Reduce())
}

func TestRollingWindowReduce(t *testing.T) {
	r := NewWindow(WithSize(4), WithInterval(duration))
	for i := 1; i <= 4; i++ {
		r.Add(int64(i * 10))
		elapse()
	}
	// 第一个桶过期
	assert.Equal(t, int64(90), r.Reduce())
}

func elapse() {
	time.Sleep(duration)
}

```
## go-zero中滑动窗口如何实现
> 基于 go-zero v1.6.0

如果前面的滑动窗口能理解，go-zero 中的工业级的滑动窗口也就不难了
#### 结构体定义
go-zero 中包含三个结构体定义：

- RollingWindow 最外层滑动窗口，用于用户调用添加统计值和获取统计结果
- window 具体的滑动窗口，即真正实现滑动窗口的环形队列
- Bucket 滑动窗口中的桶，一个桶标识一个时间刻度，其内存储统计值

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1702311615051-aad74955-9b93-4682-8e00-8f5d30bbfd6d.png#averageHue=%23f9f9f9&clientId=u04d11255-8e72-4&from=paste&height=294&id=u96d82094&originHeight=441&originWidth=1198&originalType=binary&ratio=1.5&rotation=0&showTitle=false&size=54527&status=done&style=none&taskId=u3e9ae617-7a78-4f84-a583-76ac4693c13&title=&width=798.6666666666666)
```go
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


// 滑动窗口
type RollingWindow struct {
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
```
#### Add
向滑动窗口中添加数据，做了以下操作：

1. 根据当前时间距离上次添加时间经过了多少刻度，重新算偏移量
2. 中间经过的刻度，就是过期的桶，清空过期的桶中的数据
3. 更新偏移量，使用当前时间更新上次添加时间
4. 向当前桶中添加统计数据
```go
// Add 将 v 添加到当前滑动窗口的桶中
func (rw *RollingWindow) Add(v float64) {
	rw.lock.Lock()
	defer rw.lock.Unlock()

	// 更新偏移量
	rw.updateOffset()
	// 向当前窗口的桶中添加数据
	rw.win.add(rw.offset, v)
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
```
![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1702312347192-354f85a7-5d25-4483-af39-34c13576f526.png#averageHue=%23f8f8f8&clientId=u04d11255-8e72-4&from=paste&height=457&id=ud18dc54e&originHeight=686&originWidth=1680&originalType=binary&ratio=1.5&rotation=0&showTitle=false&size=120839&status=done&style=none&taskId=u36397602-9d52-4c7f-9de1-a7f59a2a89f&title=&width=1120)
#### Reduce
统计当前桶中数据，做了以下操作：

1. 根据当前时间距离上次添加时间经过了多少刻度，算偏移量
2. 在未过期的桶中，执行 fn 函数
3. 特别注意，当前桶的下一个桶永远是第一个过期的桶
```go
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
```

## 源码 & 参考
滑动窗口源码地址：https://github.com/callmePicacho/rollingwindow

[源码解读四：滑动窗口数据统计](https://xiaozhuanlan.com/topic/3417605982)  
[Sentinel 基于滑动窗口的实时指标数据统计](https://zhuanlan.zhihu.com/p/612284419)  
[限流-滑动时间窗口](https://www.bilibili.com/video/BV1sZ4y1v7b3/?spm_id_from=333.880.my_history.page.click)  
[go-zero服务治理-自适应熔断器](https://juejin.cn/post/7028536954262126605)
