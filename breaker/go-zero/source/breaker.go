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
