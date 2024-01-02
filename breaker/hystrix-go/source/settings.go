package hystrix

import (
	"sync"
	"time"
)

var (
	//DefaultTimeout 命令执行超时时间，单位 ms，默认 1s
	DefaultTimeout = 1000
	// DefaultMaxConcurrent 最大并发数，默认 10
	DefaultMaxConcurrent = 10
	// DefaultVolumeThreshold 能触发熔断器打开的最小请求数，默认 20
	DefaultVolumeThreshold = 20
	// DefaultSleepWindow 熔断器打开后，多久之后可以进入半打开状态，单位 ms，默认 5s
	DefaultSleepWindow = 5000
	// DefaultErrorPercentThreshold 导致熔断器打开的错误百分比，默认 50%
	DefaultErrorPercentThreshold = 50
	// DefaultLogger log
	DefaultLogger = NoopLogger{}
)

type Settings struct {
	Timeout                time.Duration // 命令执行超时时间
	MaxConcurrentRequests  int           // 最大并发数
	RequestVolumeThreshold uint64        // 能触发熔断器打开的最小请求数，统计的时间周期是10s
	SleepWindow            time.Duration // 熔断器打开后，多久之后可以进入半打开状态
	ErrorPercentThreshold  int           // 导致熔断器打开的错误百分比
}

// CommandConfig 用于运行时设置熔断器参数
type CommandConfig struct {
	Timeout                int `json:"timeout"`
	MaxConcurrentRequests  int `json:"max_concurrent_requests"`
	RequestVolumeThreshold int `json:"request_volume_threshold"`
	SleepWindow            int `json:"sleep_window"`
	ErrorPercentThreshold  int `json:"error_percent_threshold"`
}

// 存储熔断器配置的 map
var circuitSettings map[string]*Settings

// 熔断器配置的读写锁
var settingsMutex *sync.RWMutex
var log logger

func init() {
	circuitSettings = make(map[string]*Settings)
	settingsMutex = &sync.RWMutex{}
	log = DefaultLogger
}

// Configure 应用一组熔断器配置
// key：熔断器name，value：熔断器配置项
func Configure(cmds map[string]CommandConfig) {
	for k, v := range cmds {
		ConfigureCommand(k, v)
	}
}

// ConfigureCommand 熔断器配置项设置
// name：熔断器name，config：熔断器配置项
func ConfigureCommand(name string, config CommandConfig) {
	// 写入配置，加写锁
	settingsMutex.Lock()
	defer settingsMutex.Unlock()

	timeout := DefaultTimeout
	if config.Timeout != 0 {
		timeout = config.Timeout
	}

	max := DefaultMaxConcurrent
	if config.MaxConcurrentRequests != 0 {
		max = config.MaxConcurrentRequests
	}

	volume := DefaultVolumeThreshold
	if config.RequestVolumeThreshold != 0 {
		volume = config.RequestVolumeThreshold
	}

	sleep := DefaultSleepWindow
	if config.SleepWindow != 0 {
		sleep = config.SleepWindow
	}

	errorPercent := DefaultErrorPercentThreshold
	if config.ErrorPercentThreshold != 0 {
		errorPercent = config.ErrorPercentThreshold
	}

	// 有配置项，则使用传入配置项，没有则使用默认配置
	circuitSettings[name] = &Settings{
		Timeout:                time.Duration(timeout) * time.Millisecond,
		MaxConcurrentRequests:  max,
		RequestVolumeThreshold: uint64(volume),
		SleepWindow:            time.Duration(sleep) * time.Millisecond,
		ErrorPercentThreshold:  errorPercent,
	}
}

// 获取熔断器name的配置
func getSettings(name string) *Settings {
	// 读锁
	settingsMutex.RLock()
	s, exists := circuitSettings[name]
	settingsMutex.RUnlock()

	// 如果不存在配置项，使用默认配置项设置后返回
	if !exists {
		ConfigureCommand(name, CommandConfig{})
		s = getSettings(name)
	}

	// 如果已存在配置项，直接返回
	return s
}

// GetCircuitSettings 获取所有熔断器配置
func GetCircuitSettings() map[string]*Settings {
	copy := make(map[string]*Settings)

	// 加读锁从map中复制一份，避免外部调用后修改作用到运行中的熔断器配置项
	settingsMutex.RLock()
	for key, val := range circuitSettings {
		copy[key] = val
	}
	settingsMutex.RUnlock()

	return copy
}

// SetLogger 设置logger
func SetLogger(l logger) {
	log = l
}
