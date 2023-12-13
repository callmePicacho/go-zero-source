package bloom

import (
	"context"
	"errors"
	"strconv"

	"github.com/zeromicro/go-zero/core/hash"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// 固定使用 14 个 hash 函数
const maps = 14

var (
	// ErrTooLargeOffset 位偏移量过大
	ErrTooLargeOffset = errors.New("too large offset")

	// setScript 将指定偏移量数组对应二进制值全置为 1
	// KEYS[1] 布隆过滤器的 key
	// ARGV 偏移量数组
	setScript = redis.NewScript(`
    for _, offset in ipairs(ARGV) do
    	redis.call("setbit", KEYS[1], offset, 1)
    end
`)

	// testScript 检查指定偏移位置对应二进制是否全为 1
	// KEYS[1] 布隆过滤器的 key
	// ARGV 偏移量数组
	testScript = redis.NewScript(`
    for _, offset in ipairs(ARGV) do
    	if tonumber(redis.call("getbit", KEYS[1], offset)) == 0 then
    		return false
    	end
    end
    return true
`)
)

type (
	// Filter 定义布隆过滤器结构体
	Filter struct {
		bits   uint           // bitmap 使用到的位数
		bitSet bitSetProvider // bitmap 操作接口
	}

	// bitSetProvider 定义 bitmap 操作接口
	bitSetProvider interface {
		// check 检查 offsets 数组在 bitmap 中对应的位值是否全部为 1
		check(ctx context.Context, offsets []uint) (bool, error)
		// set 设置 offsets 数组在 bitmap 中对应的位值
		set(ctx context.Context, offsets []uint) error
	}
)

// Add 将data添加到bitmap中
func (f *Filter) Add(data []byte) error {
	return f.AddCtx(context.Background(), data)
}

// AddCtx 将data添加到bitmap中
func (f *Filter) AddCtx(ctx context.Context, data []byte) error {
	// 计算 data 对应的 hash 值，返回对应偏移量数组
	locations := f.getLocations(data)
	// 将 offsets 数组在 bitmap 中对应的位值设置为 1
	return f.bitSet.set(ctx, locations)
}

// Exists 检查data是否存在bitmap中，如果存在，返回true
func (f *Filter) Exists(data []byte) (bool, error) {
	return f.ExistsCtx(context.Background(), data)
}

// ExistsCtx 检查data是否存在bitmap中，如果存在，返回true
func (f *Filter) ExistsCtx(ctx context.Context, data []byte) (bool, error) {
	// 计算 data 对应的 hash 值，返回对应偏移量数组
	locations := f.getLocations(data)
	// 检查 offsets 数组在 bitmap 中对应的位值是否全部为 0
	isSet, err := f.bitSet.check(ctx, locations)
	if err != nil {
		return false, err
	}

	return isSet, nil
}

// getLocations 计算 data 对应的 hash 值，返回对应偏移量数组
func (f *Filter) getLocations(data []byte) []uint {
	locations := make([]uint, maps)
	for i := uint(0); i < maps; i++ {
		// 每次向data追加一个当前索引，一并计算 hash 值
		hashValue := hash.Hash(append(data, byte(i)))
		// 计算 hash 值对应的偏移量，取余确保在 bitmap 范围内
		locations[i] = uint(hashValue % uint64(f.bits))
	}

	return locations
}

// redis bitmap
type redisBitSet struct {
	store *redis.Redis // redis客户端
	key   string       // bitmap 的 key
	bits  uint         // bitmap 长度
}

func newRedisBitSet(store *redis.Redis, key string, bits uint) *redisBitSet {
	return &redisBitSet{
		store: store,
		key:   key,
		bits:  bits,
	}
}

// buildOffsetArgs 将偏移量数组转换为 redis 脚本所需的字符串数组
func (r *redisBitSet) buildOffsetArgs(offsets []uint) ([]string, error) {
	var args []string

	for _, offset := range offsets {
		// 偏移量过大
		if offset >= r.bits {
			return nil, ErrTooLargeOffset
		}

		args = append(args, strconv.FormatUint(uint64(offset), 10))
	}

	return args, nil
}

// check 检查 offsets 数组在 bitmap 中对应的位值是否全部为 1
func (r *redisBitSet) check(ctx context.Context, offsets []uint) (bool, error) {
	// 偏移量参数转换
	args, err := r.buildOffsetArgs(offsets)
	if err != nil {
		return false, err
	}

	// redis 执行检查 lua 脚本，返回 1 表示存在，0 表示不存在
	resp, err := r.store.ScriptRunCtx(ctx, testScript, []string{r.key}, args)
	// 当 key 不存在，会返回 redis.Nil，这是预期合法场景，需要特殊处理
	if err == redis.Nil {
		return false, nil
	} else if err != nil {
		return false, err
	}

	// redis 返回值转换
	exists, ok := resp.(int64)
	if !ok {
		return false, nil
	}

	return exists == 1, nil
}

// del 删除 bitmap
func (r *redisBitSet) del() error {
	_, err := r.store.Del(r.key)
	return err
}

// expire 设置过期时间
func (r *redisBitSet) expire(seconds int) error {
	return r.store.Expire(r.key, seconds)
}

// set 设置 offsets 数组在 bitmap 中对应的位值
func (r *redisBitSet) set(ctx context.Context, offsets []uint) error {
	// 偏移量参数转换
	args, err := r.buildOffsetArgs(offsets)
	if err != nil {
		return err
	}

	// redis 执行检查 lua 脚本，设置对应偏移量位置值为 1
	_, err = r.store.ScriptRunCtx(ctx, setScript, []string{r.key}, args)
	// 特殊处理 key 不存在的场景
	if err == redis.Nil {
		return nil
	}

	return err
}
