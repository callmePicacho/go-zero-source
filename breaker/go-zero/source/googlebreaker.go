package breaker

import (
	"math"
	"time"

	"github.com/zeromicro/go-zero/core/collection"
	"github.com/zeromicro/go-zero/core/mathx"
)

const (
	// 250ms for bucket duration
	window     = time.Second * 10
	buckets    = 40
	k          = 1.5
	protection = 5
)

// googleBreaker is a netflixBreaker pattern from google.
// see Client-Side Throttling section in https://landing.google.com/sre/sre-book/chapters/handling-overload/
type googleBreaker struct {
	k     float64                   // google 算法中的倍率，默认 1.5
	stat  *collection.RollingWindow // 时间轮
	proba *mathx.Proba              //  动态概率
}

// 创建一个 google 自适应算法的熔断器
func newGoogleBreaker() *googleBreaker {
	// 总时间 10s，每个桶 250ms
	bucketDuration := time.Duration(int64(window) / int64(buckets))
	// 初始化 40 个桶，每个桶 250ms 的时间轮
	st := collection.NewRollingWindow(buckets, bucketDuration)
	return &googleBreaker{
		stat:  st,
		k:     k,
		proba: mathx.NewProba(),
	}
}

// 自适应熔断算法
func (b *googleBreaker) accept() error {
	// 请求接受量和请求总量
	accepts, total := b.history()
	// 计算丢弃请求概率
	weightedAccepts := b.k * float64(accepts)
	// https://landing.google.com/sre/sre-book/chapters/handling-overload/#eq2101
	dropRatio := math.Max(0, (float64(total-protection)-weightedAccepts)/float64(total+1))
	if dropRatio <= 0 {
		return nil
	}

	// 动态判断是否触发熔断
	if b.proba.TrueOnProba(dropRatio) {
		return ErrServiceUnavailable
	}

	return nil
}

func (b *googleBreaker) allow() (internalPromise, error) {
	// 判断是否触发熔断
	if err := b.accept(); err != nil {
		return nil, err
	}

	return googlePromise{
		b: b,
	}, nil
}

func (b *googleBreaker) doReq(req func() error, fallback func(err error) error, acceptable Acceptable) error {
	// 判断是否触发熔断
	if err := b.accept(); err != nil {
		// 如果传了 fallback 函数，则执行 fallback 函数
		if fallback != nil {
			return fallback(err)
		}

		return err
	}

	defer func() {
		if e := recover(); e != nil {
			// 标记失败
			b.markFailure()
			panic(e)
		}
	}()

	// 执行真正的调用
	err := req()
	// 如果接受，则标记成功
	if acceptable(err) {
		b.markSuccess()
	} else {
		// 否则标记失败
		b.markFailure()
	}

	return err
}

// 标记成功，请求总数+1，接受数+1
func (b *googleBreaker) markSuccess() {
	b.stat.Add(1)
}

// 标记失败，请求总数+1，接受数+0
func (b *googleBreaker) markFailure() {
	b.stat.Add(0)
}

// 返回请求接受数量和请求总量
func (b *googleBreaker) history() (accepts, total int64) {
	b.stat.Reduce(func(b *collection.Bucket) {
		accepts += int64(b.Sum)
		total += b.Count
	})

	return
}

type googlePromise struct {
	b *googleBreaker
}

// Accept 接受就是标记成功一次
func (p googlePromise) Accept() {
	p.b.markSuccess()
}

// Reject 拒绝就是标记失败一次
func (p googlePromise) Reject() {
	p.b.markFailure()
}
