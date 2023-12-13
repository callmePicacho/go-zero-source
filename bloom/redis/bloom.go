package bloom

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/spaolacci/murmur3"
	"strconv"
)

var (
	// 批量设置 bitmap 的脚本
	setScript = redis.NewScript(`
	for _, offset in ipairs(ARGV) do
		redis.call("setbit", KEYS[1], offset, 1)
	end
`)

	// 判断 bitmap 指定位是否存在 0
	getScript = redis.NewScript(`
	for _, offset in ipairs(ARGV) do
		if redis.call("getbit", KEYS[1], offset) == 0 then
			return 0
		end
	end
	return 1
`)
)

type RedisClient struct {
	*redis.Client
}

func NewRedisConf(addr, pass, username string, db int) *RedisClient {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pass,
		DB:       db,
		Username: username,
	})

	_, err := client.Ping(context.TODO()).Result()
	if err != nil {
		panic(err)
	}

	redisClient := &RedisClient{
		client,
	}
	return redisClient
}

type Filter struct {
	bitmap []uint64
	k      int32 // hash 函数个数
	m      int32 // bitmap 的长度
	client *RedisClient
}

// NewFilter 获取Redis布隆过滤器
func NewFilter(m, k int32, client *RedisClient) *Filter {
	return &Filter{
		bitmap: make([]uint64, m/64+1),
		k:      k,
		m:      m,
		client: client,
	}
}

// Set 将元素添加到布隆过滤器中
// key：redis 键
// val：元素值
func (f *Filter) Set(ctx context.Context, key, val string) error {
	// 获取需要将 bitmap 置 1 的位下标
	locations := f.getLocations([]byte(val))
	// 存入 Redis
	return f.set(ctx, key, locations)
}

// Exists 判断元素是否存在布隆过滤器中
func (f *Filter) Exists(ctx context.Context, key string, val string) (bool, error) {
	// 获取需要判断 bitmap 是否为 1 的位下标
	locations := f.getLocations([]byte(val))
	// 从 Redis 获取 bitmap
	return f.exists(ctx, key, locations)
}

// 将指定位的 bitmap 设置为 1
func (f *Filter) set(ctx context.Context, key string, locations []int32) error {
	args := f.buildOffsetArgs(locations)

	_, err := setScript.Eval(ctx, f.client, []string{key}, args).Result()
	if err == redis.Nil {
		return nil
	}

	return err
}

// 判断 bitmap 指定位是否存在 0
func (f *Filter) exists(ctx context.Context, key string, locations []int32) (bool, error) {
	args := f.buildOffsetArgs(locations)

	resp, err := getScript.Eval(ctx, f.client, []string{key}, args).Result()
	if err == redis.Nil {
		return false, nil
	} else if err != nil {
		return false, err
	}

	exists, ok := resp.(int64)
	if !ok {
		return false, nil
	}

	return exists == 1, err
}

// 将 []int32 转换成 []string 数组
func (f *Filter) buildOffsetArgs(locations []int32) []string {
	args := make([]string, len(locations))
	for i, offset := range locations {
		args[i] = strconv.FormatInt(int64(offset), 10)
	}

	return args
}

// getLocations 使用 k 个 hash 函数，得到将要设置为 1 的位下标
func (f *Filter) getLocations(data []byte) []int32 {
	locations := make([]int32, f.k)
	for i := 0; int32(i) < f.k; i++ {
		// 使用不同序列号撒入 data，得到不同 hash
		hash := Hash(append(data, byte(i)))
		// 取余，确保得到的结果落在 bitmap 中
		locations[i] = int32(hash % uint64(f.m))
	}

	return locations
}

// Hash 将输入data转换为uint64的hash值
func Hash(data []byte) uint64 {
	return murmur3.Sum64(data)
}
