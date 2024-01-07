package main

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"go-zero-source/breaker/breaker/source"
	"math/rand"
	"net/http"
)

var brk = source.NewGoogleBreaker()

func CircuitBreakerWrapper(c *gin.Context) {
	err := brk.Do(func() error {
		// 放行
		c.Next()

		// 记录错误
		code := c.Writer.Status()
		if code != http.StatusOK {
			return errors.New(fmt.Sprintf("brk status code %d", code))
		}

		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
}

func main() {
	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		if rand.Intn(10) < 5 {
			c.JSON(http.StatusOK, gin.H{"msg": "ok"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"msg": "err"})
	}, CircuitBreakerWrapper)

	r.Run()
}
