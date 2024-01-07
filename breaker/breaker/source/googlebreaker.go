package source

import (
	"errors"
	"github.com/zeromicro/go-zero/core/collection"
	"math"
	"math/rand"
	"time"
)

var (
	// ErrBreakerOpen 熔断器打开错误
	ErrBreakerOpen = errors.New("circuit breaker is open")

	// 倍率
	k = 1.5
	// 时间轮总共记录10s
	window = time.Second * 10
	// 总共40个桶，等于每个桶250ms
	buckets = 40
)

// GoogleBreaker Google 算法的熔断器
// https://landing.google.com/sre/sre-book/chapters/handling-overload/
type GoogleBreaker struct {
	k             float64                   // 倍率，默认 1.5
	rollingWindow *collection.RollingWindow // 时间轮，负责收集错误率
}

func NewGoogleBreaker() *GoogleBreaker {
	return &GoogleBreaker{
		k:             k,
		rollingWindow: collection.NewRollingWindow(buckets, time.Duration(int64(window)/int64(buckets))),
	}
}

// Do 传入请求，由熔断器判断是否执行
func (b *GoogleBreaker) Do(req func() error) error {
	// 判断是否触发熔断
	if err := b.accept(); err != nil {
		return err
	}

	// 执行调用
	err := req()
	if err != nil {
		b.markFail() // 标记失败
	} else {
		b.markSuccess() // 标记成功
	}

	return err
}

// 自适应熔断算法，计算请求是否被接收
func (b *GoogleBreaker) accept() error {
	// 获取请求总数和请求成功次数
	accepts, requests := b.history()
	// 计算请求丢弃概率
	dropRatio := math.Max(0, (float64(requests)-b.k*float64(accepts))/(float64(requests)+1))
	// 动态判断是否触发熔断
	if rand.Float64() < dropRatio {
		return ErrBreakerOpen
	}

	return nil
}

// 获取统计信息，请求的总数和请求的成功次数
func (b *GoogleBreaker) history() (accepts, requests int64) {
	b.rollingWindow.Reduce(func(b *collection.Bucket) {
		// 请求成功数
		accepts += int64(b.Sum)
		// 请求总数
		requests += b.Count
	})

	return
}

// 标记成功，请求总数、请求成功数+1
func (b *GoogleBreaker) markSuccess() {
	b.rollingWindow.Add(1)
}

// 标记失败，请求总数+1
func (b *GoogleBreaker) markFail() {
	b.rollingWindow.Add(0)
}
