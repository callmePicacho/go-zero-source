package main

import (
	"errors"
	"fmt"
	"github.com/afex/hystrix-go/hystrix"
	"github.com/gin-gonic/gin"
	"net/http"
)

var BreakerName = "ping_breaker"

func init() {
	// 设置 hystrix-go 熔断器配置项
	hystrix.ConfigureCommand(BreakerName, hystrix.CommandConfig{
		// 执行 run 方法的超时时间：3s
		Timeout: 3 * 1000,
		// run 方法的最大并发量
		MaxConcurrentRequests: 10,
		// 触发开启熔断的最小请求数
		RequestVolumeThreshold: 50,
		// 熔断器开启后，多久之后进入半开状态
		SleepWindow: 5 * 1000,
		// 当错误率超过该阈值，且请求量大于等于RequestVolumeThreshold，熔断器打开
		ErrorPercentThreshold: 50,
	})
}

func CircuitBreakerWrapper(ctx *gin.Context) {
	run := func() error {
		// 放行
		ctx.Next()

		// 记录错误
		code := ctx.Writer.Status()
		if code != http.StatusOK {
			return errors.New(fmt.Sprintf("status code %d", code))
		}

		return nil
	}

	fallback := func(err error) error {
		if err != nil {
			fmt.Println("breaker err,", err)
			// 返回熔断错误
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"msg": err.Error(),
			})
		}

		return nil
	}

	// 熔断器关闭时，执行 run，熔断器打开时，执行 fallback
	// 熔断器入口
	hystrix.Do(BreakerName, run, fallback)
}

// hystrix-go Example
func main() {
	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		_, err := http.Get("https://www.baidu.com")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"msg": "success"})
	}, CircuitBreakerWrapper)

	r.Run()
}
