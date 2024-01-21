package source

import (
	"go-zero-source/hash/hash/source/redis"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	keySize = 20
)

// BenchmarkConsistentHashGet-16            4849216               208.8 ns/op
func BenchmarkConsistentHashGet(b *testing.B) {
	ch := NewConsistentHash()
	for i := 0; i < keySize; i++ {
		ch.Add("localhost:" + strconv.Itoa(i))
	}

	for i := 0; i < b.N; i++ {
		ch.Get(strconv.Itoa(i))
	}
}

// BenchmarkConsistentHashRedisGet-16           603           3138432 ns/op
func BenchmarkConsistentHashRedisGet(b *testing.B) {
	redisCh := redis.NewZSetHashRing("BenchmarkConsistentHashRedisGet", "localhost:6379", "")
	ch := NewCustomConsistentHash(redisCh, redis.Hash, minReplicas)
	for i := 0; i < keySize; i++ {
		ch.Add("localhost:" + strconv.Itoa(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Get(strconv.Itoa(i))
	}
}

func TestConsistentHash_Remove(t *testing.T) {
	ch := NewConsistentHash()

	ch.Add("first")
	for i := 0; i < 100; i++ {
		val, ok := ch.Get(strconv.Itoa(i))
		assert.True(t, ok)
		assert.Equal(t, "first", val)
	}

	ch.Add("second")
	ch.Remove("first")

	for i := 0; i < 100; i++ {
		val, ok := ch.Get(strconv.Itoa(i))
		assert.True(t, ok)
		assert.Equal(t, "second", val)
	}
}

func TestConsistentHashRedis(t *testing.T) {
	redisCh := redis.NewZSetHashRing("hashRing", "localhost:6379", "")
	ch := NewCustomConsistentHash(redisCh, redis.Hash, minReplicas)

	ch.Add("first")
	for i := 0; i < 100; i++ {
		val, ok := ch.Get(strconv.Itoa(i))
		assert.True(t, ok)
		assert.Equal(t, "first", val)
	}

	ch.Add("second")
	ch.Remove("first")

	for i := 0; i < 100; i++ {
		val, ok := ch.Get(strconv.Itoa(i))
		assert.True(t, ok)
		assert.Equal(t, "second", val)
	}
}
