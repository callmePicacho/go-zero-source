## 为什么需要熔断器
### 雪崩效应
在微服务中，各种服务互相依赖。
比如评论服务依赖审核服务，审核服务依赖反垃圾服务，当评论服务调用审核服务时，审核服务又调用反垃圾服务，这时如果反垃圾服务超时了，由于审核服务依赖反垃圾服务，反垃圾服务的超时会导致审核服务逻辑一直等待，而这时评论服务又一直在调用审核服务，审核服务对反垃圾服务的调用会占用越来越多的资源，审核服务就有可能因为堆积了大量请求而导致服务宕机，进而引起崩溃，导致"雪崩效应"。

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1699397156432-8efed053-4b37-4552-8a3d-5a2a89755a38.png#averageHue=%23f8d6d3&clientId=u19cd0b1f-3520-4&from=paste&height=428&id=ue90dd5dd&originHeight=856&originWidth=1572&originalType=binary&ratio=1.5&rotation=0&showTitle=false&size=119627&status=done&style=none&taskId=ua8b9101c-5469-4692-8055-63cb72f292a&title=&width=786)
### 如何避免雪崩效应
##### 服务熔断
熔断是**调用方**进行自我保护的一种手段，当一个服务作为调用方调用另一个服务时，为了防止被调用方不可用或响应时间太长，熔断该节点的调用，进行服务降级，**快速返回错误的响应信息**。当检测到该节点服务调用正常后，服务调用链路。
> 总结：**调用方**为保护自己不被下游服务拖垮，快速返回错误响应。

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1699397613908-8fee5fbb-4c3a-422c-aaf6-863930a51d7a.png#averageHue=%23f9f6f6&clientId=u19cd0b1f-3520-4&from=paste&height=491&id=u64d83d17&originHeight=737&originWidth=1894&originalType=binary&ratio=1.5&rotation=0&showTitle=false&size=132174&status=done&style=none&taskId=u3bb2df2c-7d2f-44a2-a011-311ba245d7a&title=&width=1262.6666666666667)
##### 服务降级
降级是**被调用方**防止因自身资源不足导致过载的自我保护机制，对某些服务不再进行调用，而是直接返回默认值。
> 总结：**被调用方**防止自身崩溃，对某些请求快速返回。

##### 服务限流
限流是指**调用方**针对接口调用频率进行限制，以免超过承载上限拖垮下游系统。
> 总结：**调用方**对调用频率进行限制

## 熔断器的工作原理
熔断机制实际上是参考了日常生活中保险丝的保护机制，当电路超负荷运载时，保险丝会自动断开，从而保证电路中的电器不受损害。而服务治理中的熔断机制，指的是发起服务调用时，如果被调用方返回的错误率超过一定阈值，那么后续的请求将不会真正发起，而是在调用方直接返回错误。

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1699761809493-4bedebe2-48be-471b-95aa-0986bca22d1f.png#averageHue=%23f9f9f9&clientId=u7fc8716e-5518-4&from=paste&height=405&id=u85b63993&originHeight=405&originWidth=693&originalType=binary&ratio=1&rotation=0&showTitle=false&size=25666&status=done&style=none&taskId=ud74361c1-fcd9-4c9f-acaf-4e9aa294eaa&title=&width=693)

在这种模式下，服务调用方实际是为每一个服务调用维护了一个状态机，包含三种状态：

- 关闭（closed）状态：默认状态。请求能正常调用，同时统计在窗口时间成功和失败的次数，如果错误率达到阈值，将会进入**打开**状态。
- 打开（open）状态：该状态下会直接返回错误。超时时间后，切换为**半打开**状态。
- 半打开（half-open）状态：允许一部分请求正常调用，并统计成功和失败次数，如果成功次数达到阈值，进入**关闭**状态，否则再次进入**打开**状态。半打开状态存在的意义在于可以有效防止正在恢复中的服务突然被大量请求再次打垮。
> 熔断器默认是关闭的，当失败次数多了，切换到打开状态，打开状态直接返回错误，一段时间后切换到半打开状态，半打开状态允许尝试性调用一些下游请求，尝试没问题再转回关闭状态。

## 需求分析
思考熔断器如何实现，其中几个关键点：

1. 熔断器是通过指标来进行状态转换，那么我们应当如何统计指标？
2. 熔断器需要统计的指标包括哪些？
3. 熔断器应当有哪些需要设置的值？
4. 如何定义调用是"成功"还是"失败"？
5. 如何快速失败？
6. 熔断器是否需要线程安全？

试着回答一下上面的问题：

1. 可以通过"滑动窗口"来存储统计数据，每个窗口代表一个时间刻度，存储着该时间刻度内的统计数据。每隔一段时间从滑动窗口获取一次统计指标，时间复杂度为 O(1)
2. 需要统计的指标包括：请求总数、成功次数和失败次数
3. 回顾熔断器流转的整个过程，需要设置的参数包括：
   1. 故障率阈值：状态关闭时，故障率达到此阈值，熔断器从关闭状态转为开启状态
   2. 打开状态持续时间：打开状态时，持续多长时间后，熔断器从打开状态转为半打开状态
   3. 半打开状态持续时间：半打开状态时，持续多长时间后，熔断器允许进行尝试性调用
   4. 半打开状态尝试调用时，实际进行调用的比例：半打开状态时，当熔断器允许进行尝试性调用，按照实际流量的比例进行下游服务调用
   5. 半打开状态尝试调用持续时间：半打开状态时，熔断器尝试进行下游服务调用的持续时间
   6. 半打开状态成功请求阈值：半打开状态时，尝试调用时，成功次数达到该阈值，熔断器从半打开状态转为关闭状态。
4. 可以提供一个自定义函数，由业务方来决定请求是"成功"还是"失败"
5. 同样也是提供自定义函数，由业务方决定请求如何"快速失败"
6. 不可能为每个接口提供一个熔断器，所以一定是需要线程安全的
### 自适应熔断器
前面简单的需求分析，需要设置六个参数项，对于经验不够丰富的开发人员，这些参数设置多少合适心里其实并没有底。
《Google SRE》提供了一种自适应熔断算法算法，计算丢弃请求的概率：

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1702560851290-e464d233-5e94-4935-b72c-38ec37b9ac22.png#averageHue=%23f3f3f3&clientId=u6779154d-de19-4&from=paste&height=89&id=u6b5e3fb1&originHeight=89&originWidth=348&originalType=binary&ratio=1&rotation=0&showTitle=false&size=3926&status=done&style=none&taskId=u332dece0-9d36-4850-9e66-e647715031e&title=&width=348)

具体参数解释如下：

- 请求数量（requests）：调用方发起的数量总和
- 请求接受数量（accepts）：被调用方正常处理的请求数量
- K，算法敏感度，K 越小越容易丢弃请求，通常使用 2

算法解释如下：

1. 正常情况下，requests = accepts，所以概率为 0
2. 随着正常请求减少，当 requests == K * accepts 继续请求时，概率会逐渐比 0 大，按照概率逐渐丢弃请求，当正常请求完全失败，仍然存在很小的概率请求。
3. 随着正常请求增加，accepts 和 requests 同时增加，但是 K*accepts 比 requests 增加更快，所以概率逐渐归 0，彻底关闭熔断器。
## 熔断器实现
技术选型，前面提到，统计使用滑动窗口，是否丢弃请求使用 Google SRE 的算法，借助 go-zero 中的滑动窗口，可以很简单实现一个熔断器：
```go
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

```
## go-zero 熔断器的实现
go-zero 熔断器实现复杂一些，但是本质和自己实现的一样：

1. 使用滑动窗口统计请求信息
2. 使用Google SRE 算法计算丢弃概率

相比之下，新增了几个接口：

1. `Alloc`允许在熔断器关闭时，返回一个对象，可以调用`Accept`和`Reject`手动向熔断器上报此次请求成功或失败
2. `Do`方法族，本质都是调用 `DoWithFallbackAcceptable`：
```go
DoWithFallbackAcceptable(req func() error, fallback func(err error) error, acceptable Acceptable) error
```

- 接收 `req` 作为实际调用函数
- 当熔断器打开，调用 `fallback`
- 根据 `req` 执行结果，决定此次请求是否成功的 `acceptable`函数
```go
package breaker

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/mathx"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/stat"
	"github.com/zeromicro/go-zero/core/stringx"
)

const (
	numHistoryReasons = 5
	timeFormat        = "15:04:05"
)

// ErrServiceUnavailable 熔断器打开时返回错误
var ErrServiceUnavailable = errors.New("circuit breaker is open")

type (
	// Acceptable 检查函数是否可以被接受
	Acceptable func(err error) bool

	// Breaker 熔断器接口
	Breaker interface {
		// Name 返回断路器的名称
		Name() string

		// Allow 用于判断请求是否被允许，如果允许，返回 Promise 接口，如果不允许，返回 ErrServiceUnavailable 错误
		// 调用方在成功或失败时分别调用 Promise 的 Accept 或 Reject 方法
		Allow() (Promise, error)

		// Do 方法用于执行请求，如果请求被允许，则执行 req 方法，并将结果返回；如果请求被拒绝，则直接返回错误
		Do(req func() error) error

		// DoWithAcceptable 方法与Do方法类似，但它多传入一个 Acceptable 函数，用于判断请求是否为成功调用
		DoWithAcceptable(req func() error, acceptable Acceptable) error

		// DoWithFallback 方法与Do方法类似，但它多传入一个 fallback 函数，用于在请求被拒绝时执行回退逻辑
		DoWithFallback(req func() error, fallback func(err error) error) error

		// DoWithFallbackAcceptable 方法与 Do 方法类似，但是同时多传了 fallback 和 acceptable 函数
		DoWithFallbackAcceptable(req func() error, fallback func(err error) error, acceptable Acceptable) error
	}

	// Option 定义了 Breaker 的自定义方法
	Option func(breaker *circuitBreaker)

	// Promise 接口定义了 Breaker.Allow 方法返回的 Promise 接口
	Promise interface {
		// Accept 告知熔断器请求成功
		Accept()
		// Reject 告知熔断器请求失败
		Reject(reason string)
	}

	internalPromise interface {
		Accept()
		Reject()
	}

	// 熔断器对象
	circuitBreaker struct {
		name string
		throttle
	}

	//
	internalThrottle interface {
		allow() (internalPromise, error)
		doReq(req func() error, fallback func(err error) error, acceptable Acceptable) error
	}

	//
	throttle interface {
		allow() (Promise, error)
		doReq(req func() error, fallback func(err error) error, acceptable Acceptable) error
	}
)

// NewBreaker 初始化熔断器对象
func NewBreaker(opts ...Option) Breaker {
	var b circuitBreaker
	for _, opt := range opts {
		opt(&b)
	}
	// 如果没有传名称，默认生成一个随机名称
	if len(b.name) == 0 {
		b.name = stringx.Rand()
	}

	// 初始化 google 算法
	b.throttle = newLoggedThrottle(b.name, newGoogleBreaker())

	return &b
}

func (cb *circuitBreaker) Allow() (Promise, error) {
	return cb.throttle.allow()
}

func (cb *circuitBreaker) Do(req func() error) error {
	return cb.throttle.doReq(req, nil, defaultAcceptable)
}

func (cb *circuitBreaker) DoWithAcceptable(req func() error, acceptable Acceptable) error {
	return cb.throttle.doReq(req, nil, acceptable)
}

func (cb *circuitBreaker) DoWithFallback(req func() error, fallback func(err error) error) error {
	return cb.throttle.doReq(req, fallback, defaultAcceptable)
}

func (cb *circuitBreaker) DoWithFallbackAcceptable(req func() error, fallback func(err error) error,
	acceptable Acceptable) error {
	return cb.throttle.doReq(req, fallback, acceptable)
}

func (cb *circuitBreaker) Name() string {
	return cb.name
}

// WithName returns a function to set the name of a Breaker.
func WithName(name string) Option {
	return func(b *circuitBreaker) {
		b.name = name
	}
}

// 默认的 Acceptable 函数，判断 err 是否为 nil
func defaultAcceptable(err error) bool {
	return err == nil
}

type loggedThrottle struct {
	name string
	internalThrottle
	errWin *errorWindow
}

func newLoggedThrottle(name string, t internalThrottle) loggedThrottle {
	return loggedThrottle{
		name:             name,
		internalThrottle: t,
		errWin:           new(errorWindow),
	}
}

// 判断是否触发熔断
func (lt loggedThrottle) allow() (Promise, error) {
	promise, err := lt.internalThrottle.allow()
	return promiseWithReason{
		promise: promise,
		errWin:  lt.errWin,
	}, lt.logError(err)
}

func (lt loggedThrottle) doReq(req func() error, fallback func(err error) error, acceptable Acceptable) error {
	return lt.logError(lt.internalThrottle.doReq(req, fallback, func(err error) bool {
		accept := acceptable(err)
		if !accept && err != nil {
			lt.errWin.add(err.Error())
		}
		return accept
	}))
}

func (lt loggedThrottle) logError(err error) error {
	// 如果是熔断打开错误
	if errors.Is(err, ErrServiceUnavailable) {
		// if circuit open, not possible to have empty error window
		stat.Report(fmt.Sprintf(
			"proc(%s/%d), callee: %s, breaker is open and requests dropped\nlast errors:\n%s",
			proc.ProcessName(), proc.Pid(), lt.name, lt.errWin))
	}

	return err
}

type errorWindow struct {
	reasons [numHistoryReasons]string
	index   int
	count   int
	lock    sync.Mutex
}

func (ew *errorWindow) add(reason string) {
	ew.lock.Lock()
	ew.reasons[ew.index] = fmt.Sprintf("%s %s", time.Now().Format(timeFormat), reason)
	ew.index = (ew.index + 1) % numHistoryReasons
	ew.count = mathx.MinInt(ew.count+1, numHistoryReasons)
	ew.lock.Unlock()
}

func (ew *errorWindow) String() string {
	var reasons []string

	ew.lock.Lock()
	// reverse order
	for i := ew.index - 1; i >= ew.index-ew.count; i-- {
		reasons = append(reasons, ew.reasons[(i+numHistoryReasons)%numHistoryReasons])
	}
	ew.lock.Unlock()

	return strings.Join(reasons, "\n")
}

type promiseWithReason struct {
	promise internalPromise
	errWin  *errorWindow
}

func (p promiseWithReason) Accept() {
	p.promise.Accept()
}

func (p promiseWithReason) Reject(reason string) {
	p.errWin.add(reason)
	p.promise.Reject()
}

```
```go
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

```

参考：  
[https://zhuanlan.zhihu.com/p/369772129](https://zhuanlan.zhihu.com/p/369772129)
[https://juejin.cn/post/7028536954262126605](https://juejin.cn/post/7028536954262126605#heading-11)
[https://github.com/skyhackvip/service_breaker](https://github.com/skyhackvip/service_breaker)
[https://www.bookstack.cn/read/go-zero-1.3-zh/breaker-algorithms.md](https://www.bookstack.cn/read/go-zero-1.3-zh/breaker-algorithms.md)
[https://sre.google/sre-book/handling-overload/](https://sre.google/sre-book/handling-overload/)
[https://github.com/sony/gobreaker](https://github.com/sony/gobreaker)
[https://juejin.cn/post/7004802597332713503](https://juejin.cn/post/7004802597332713503#heading-17)
[https://github.com/afex/hystrix-go](https://github.com/afex/hystrix-go)



