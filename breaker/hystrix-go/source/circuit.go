package hystrix

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreaker is created for each ExecutorPool to track whether requests
// should be attempted, or rejected if the Health of the circuit is too low.
// CircuitBreaker 熔断器对象，
type CircuitBreaker struct {
	Name                   string        // 熔断器名称
	open                   bool          // 判断熔断器是否打开
	forceOpen              bool          // 手动触发熔断器开关
	mutex                  *sync.RWMutex // 读写锁
	openedOrLastTestedTime int64         // 记录上一次打开熔断器，或者打开状态尝试调用的时间

	executorPool *executorPool   // 令牌桶
	metrics      *metricExchange // 监控指标
}

var (
	circuitBreakersMutex *sync.RWMutex
	// 全局熔断器map，存储全部熔断器对象
	circuitBreakers map[string]*CircuitBreaker
)

// 使用 init 饿汉式初始化全局map
func init() {
	circuitBreakersMutex = &sync.RWMutex{}
	circuitBreakers = make(map[string]*CircuitBreaker)
}

// GetCircuit 返回指定name的熔断器对象，存在则直接返回，不存在则创建并返回
// 该方法是线程安全的
func GetCircuit(name string) (*CircuitBreaker, bool, error) {
	// 加读锁尝试从 map 中获取
	circuitBreakersMutex.RLock()
	_, ok := circuitBreakers[name]
	if !ok {
		// 如果 map 中没有，则先释放读锁，然后加写锁创建熔断器对象
		circuitBreakersMutex.RUnlock()
		circuitBreakersMutex.Lock()
		defer circuitBreakersMutex.Unlock()
		// double check 再次检查是否存在
		if cb, ok := circuitBreakers[name]; ok {
			return cb, false, nil
		}
		// 如果不存在，则创建熔断器对象并返回
		circuitBreakers[name] = newCircuitBreaker(name)
	} else {
		defer circuitBreakersMutex.RUnlock()
	}

	return circuitBreakers[name], !ok, nil
}

// Flush 从内存中删除全部熔断器和监控信息
func Flush() {
	// 加写锁
	circuitBreakersMutex.Lock()
	defer circuitBreakersMutex.Unlock()

	// 对于每个熔断器
	for name, cb := range circuitBreakers {
		// 清除监控数据
		cb.metrics.Reset()
		// 清除令牌桶监控数据
		cb.executorPool.Metrics.Reset()
		// 从全局map中移除
		delete(circuitBreakers, name)
	}
}

// newCircuitBreaker 根据name创建熔断器对象
func newCircuitBreaker(name string) *CircuitBreaker {
	c := &CircuitBreaker{}
	c.Name = name
	c.metrics = newMetricExchange(name)
	c.executorPool = newExecutorPool(name) // 初始化令牌桶
	c.mutex = &sync.RWMutex{}

	return c
}

// toggleForceOpen 手动强制切换熔断器状态
func (circuit *CircuitBreaker) toggleForceOpen(toggle bool) error {
	circuit, _, err := GetCircuit(circuit.Name)
	if err != nil {
		return err
	}

	circuit.forceOpen = toggle
	return nil
}

// IsOpen 检查熔断器是否开启
func (circuit *CircuitBreaker) IsOpen() bool {
	circuit.mutex.RLock()
	o := circuit.forceOpen || circuit.open
	circuit.mutex.RUnlock()

	// 如果熔断器状态为打开，或者设置状态为打开，返回 true
	if o {
		return true
	}

	// 判断 10s 内的并发总数是否超过设置的最大阈值
	if uint64(circuit.metrics.Requests().Sum(time.Now())) < getSettings(circuit.Name).RequestVolumeThreshold {
		return false
	}

	// 此时并发总数已经超过了设置的最大阈值，检查请求错误率是否超过了阈值，如果超过，则打开熔断器
	if !circuit.metrics.IsHealthy(time.Now()) {
		// 打开熔断器，记录此次打开的时间，方便之后进入半打开状态
		circuit.setOpen()
		return true
	}

	return false
}

// AllowRequest 如果能够执行请求
// 根据熔断器状态决定
// 或者半开状态下，偶尔尝试执行请求
func (circuit *CircuitBreaker) AllowRequest() bool {
	// IsOpen 根据熔断器状态决定是否允许执行请求
	// allowSingleTest 熔断器半开状态下，偶尔允许单个调用请求
	return !circuit.IsOpen() || circuit.allowSingleTest()
}

// 半开状态下，偶尔允许单个调用请求
func (circuit *CircuitBreaker) allowSingleTest() bool {
	circuit.mutex.RLock()
	defer circuit.mutex.RUnlock()

	now := time.Now().UnixNano()
	openedOrLastTestedTime := atomic.LoadInt64(&circuit.openedOrLastTestedTime)
	// 当熔断器为打开状态 && 熔断器打开状态持续时间超过阈值
	if circuit.open && now > openedOrLastTestedTime+getSettings(circuit.Name).SleepWindow.Nanoseconds() {
		// 尝试使用 CAS 更新 openedOrLastTestedTime，如果更新成功，返回 true
		swapped := atomic.CompareAndSwapInt64(&circuit.openedOrLastTestedTime, openedOrLastTestedTime, now)
		if swapped {
			log.Printf("hystrix-go: allowing single test to possibly close circuit %v", circuit.Name)
		}
		return swapped
	}

	return false
}

// 打开熔断器
// 记录此次打开熔断器的时间，方便之后半打开状态允许请求调用
func (circuit *CircuitBreaker) setOpen() {
	circuit.mutex.Lock()
	defer circuit.mutex.Unlock()

	if circuit.open {
		return
	}

	log.Printf("hystrix-go: opening circuit %v", circuit.Name)

	circuit.openedOrLastTestedTime = time.Now().UnixNano()
	circuit.open = true
}

// 关闭熔断器
// 重置熔断器状态，并清除监控数据
func (circuit *CircuitBreaker) setClose() {
	circuit.mutex.Lock()
	defer circuit.mutex.Unlock()

	if !circuit.open {
		return
	}

	log.Printf("hystrix-go: closing circuit %v", circuit.Name)

	circuit.open = false
	circuit.metrics.Reset()
}

// ReportEvent records command metrics for tracking recent error rates and exposing data to the dashboard.
// ReportEvent 将命令的监控指标记录到内存中，方便后续的错误率计算和展示
func (circuit *CircuitBreaker) ReportEvent(eventTypes []string, start time.Time, runDuration time.Duration) error {
	if len(eventTypes) == 0 {
		return fmt.Errorf("no event types sent for metrics")
	}

	circuit.mutex.RLock()
	o := circuit.open
	circuit.mutex.RUnlock()
	// 上报的状态事件是 success，且当前熔断器是开启状态，则说明下游服务正常了，可以关闭熔断器了
	if eventTypes[0] == "success" && o {
		// 关闭熔断器
		circuit.setClose()
	}

	var concurrencyInUse float64
	// 最大并发数设置大于 0
	if circuit.executorPool.Max > 0 {
		// 当前并发使用率 = 当前令牌桶中令牌的数量（可用数量） / 最大并发数
		concurrencyInUse = float64(circuit.executorPool.ActiveCount()) / float64(circuit.executorPool.Max)
	}

	select {
	// 上报
	case circuit.metrics.Updates <- &commandExecution{
		Types:            eventTypes,
		Start:            start,
		RunDuration:      runDuration,
		ConcurrencyInUse: concurrencyInUse,
	}:
	default:
		return CircuitError{Message: fmt.Sprintf("metrics channel (%v) is at capacity", circuit.Name)}
	}

	return nil
}
