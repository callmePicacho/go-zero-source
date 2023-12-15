package redis

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
)

var RedisClient *redis.Client

func init() {
	RedisClient = redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	err := RedisClient.Ping(context.Background()).Err()
	if err != nil {
		fmt.Println(err)
	}
}
