[源码地址](https://github.com/callmePicacho/go-zero-source/tree/main/bloom)
## 为什么需要布隆过滤器
在一些场景中，需要快速判断某元素是否在集合中，例如：

1. 手机号是否重复
2. 防止缓存击穿
3. 判断用户是否领过某张优惠券

针对以上常规做法为：查询数据库，数据库硬抗。
改进做法：使用内存 map 或 Redis 中的 set 数据结构，当数据规模非常大时此方案的内存容量要求可能会非常高。
此类问题其实可以抽象为：如何高效判断一个元素不在集合中？有没有更好的方案达到时间复杂度和空间复杂度的双优呢？
有！布隆过滤器！
## 什么是布隆过滤器
布隆过滤由一个 bitmap 和一系列随机映射函数组成，它不存放数据的明细内容，仅仅标识一条数据是否存在的信息，其最大的优点是拥有很良好的空间利用率和查询效率。
其工作原理为：
当一个元素被加入集合时，通过 K 个散列函数将这个元素映射到一个 bitmap 中的 K 个点，把它们置为 1。
检索时，我们只要看看这些点是不是都是 1 就知道集合中有没有它了；如果这些点有任何一个 0，则被检元素**一定**不在；如果都是 1；则被检元素**可能**存在。
简单来说就是准备一个长度为 m 的位数组并初始化所有元素为 0，用 k 个散列函数对元素进行 k 次散列运算跟 len(m) 取余得到 k 个位置并将 m 中对应位置设置为 1。

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1698754055387-b2d5e687-3012-42de-b0f9-aa4aec4ca8bd.png#averageHue=%23f4f4f4&clientId=u2a819b7a-6df7-4&from=paste&height=574&id=ud35c36ce&originHeight=581&originWidth=1104&originalType=binary&ratio=1&rotation=0&showTitle=false&size=123199&status=done&style=none&taskId=u396d5567-ddb4-4532-9bbe-b3e85ebe6e2&title=&width=1091)
## 布隆过滤器的优缺点
### 优点：

- 节省空间：相比于 map，布隆过滤器可以仅使用一个 bit 位标识一条数据是否存在
- 性能高效：插入与查询的时间均为 O(k），k 表示散列函数执行次数
### 缺点：

1. 假阳性误判
2. 无法删除
#### 假阳性
布隆过滤器判断一个数据不在集合中，那这个数据**肯定不在**集合中；布隆过滤器判断一个数据在集合中，那这个数据**不一定在**集合中。
> 说不在，肯定不在，说在可能不在。

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1698754662093-10119a1c-65f5-4618-b9c1-2083d07e0c68.png#averageHue=%23f7f7f7&clientId=u2a819b7a-6df7-4&from=paste&height=551&id=ub7af73e4&originHeight=551&originWidth=1074&originalType=binary&ratio=1&rotation=0&showTitle=false&size=133607&status=done&style=none&taskId=u7d2b34cc-26b1-4839-8b0b-9e063c1f3ff&title=&width=1074)
其中，与误判率相关的参数有：

- 位数组长度 m
- 散列函数个数 k
- 预期输入元素数量 n
- 误差率 _ε_

在创建过滤器时，可以根据预期元素数量 n 和 误差率 _ε _来估算位数组长度 m 与散列函数个数 k

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1699192564024-b7c06907-6f81-434d-b98c-fb80da60c9e8.png#averageHue=%23efefef&clientId=uc14f5b23-ac8f-4&from=paste&height=163&id=u594067ad&originHeight=244&originWidth=521&originalType=binary&ratio=1.5&rotation=0&showTitle=false&size=31786&status=done&style=none&taskId=u376d319a-ac36-45c0-bbd4-8710666220b&title=&width=347.3333333333333)

![image.png](https://cdn.nlark.com/yuque/0/2023/png/2518584/1699192551948-10fea21c-2e97-4328-ba9c-9e35cd1affac.png#averageHue=%23f1f1f1&clientId=uc14f5b23-ac8f-4&from=paste&height=108&id=u4698848a&originHeight=162&originWidth=390&originalType=binary&ratio=1.5&rotation=0&showTitle=false&size=16527&status=done&style=none&taskId=u37807979-cbb8-492f-9afb-3aa7586a911&title=&width=260)

相关参数调优可以使用：[Bloom filter calculator](https://hur.st/bloomfilter/?n=9000000&p=&m=65000000&k=6)
#### 无法删除
由于哈希碰撞的原因，bitmap 中的某些点是多个元素重复使用的，因此无法删除， bitmap 使用越久，被置为 1 的位越多，发生误判的概率就越高。极端场景下，全部位为1，针对不存在数据的误判概率为 100%。
可以通过**定期重建**的方式清除脏数据。例如我们在数据库中有全量数据，使用布隆过滤器仅用于保护数据库，可以定期使用指定范围内的数据新建一个 bitmap，然后使用 rename 的方式对老的 bitmap 进行覆盖，以此延长布隆过滤器的生命力。
> 以下两种数据结构可以解决布隆过滤器无法删除的问题，不在此文中讨论：
> - Counting Bloom Filter
> - 布谷鸟过滤器

## 本地布隆过滤器的实现
### 代码实现
```go
package bloom

import "github.com/spaolacci/murmur3"

type Filter struct {
	bitmap []uint64
	k      int32 // hash 函数个数
	m      int32 // bitmap 的长度
}

// NewFilter 获取本地布隆过滤器
func NewFilter(m, k int32) *Filter {
	return &Filter{
		bitmap: make([]uint64, m/64+1),
		k:      k,
		m:      m,
	}
}

// Set 将元素添加到布隆过滤器中
func (f *Filter) Set(val string) {
	// 获取需要将 bitmap 置 1 的位下标
	locations := f.getLocations([]byte(val))
	f.set(locations)
}

// Exists 判定元素 val 是否存在
// - 当返回 false，该元素必定不存在
// - 当返回 true，该元素并非必定存在，可能不存在（假阳性）
func (f *Filter) Exists(val string) bool {
	// 获取该 val 对应 bitmap 中哪些位下标
	locations := f.getLocations([]byte(val))
	return f.check(locations)
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

// set 将bitmap特定位置的值设置为1
func (f *Filter) set(offsets []int32) {
	for _, offset := range offsets {
		idx, bitOffset := f.calIdxAndBitOffset(offset)
		f.bitmap[idx] |= 1 << bitOffset
	}
}

// check 检查位下标对应的 bitmap，若存在值为 0，返回 false
func (f *Filter) check(offsets []int32) bool {
	for _, offset := range offsets {
		idx, bitOffset := f.calIdxAndBitOffset(offset)

		// 当某一位为 0，必定不存在
		if f.bitmap[idx]&(1<<bitOffset) == 0 {
			return false
		}
	}

	return true
}

// offset 是bitmap 的位下标，需要转换为 []uint64 数组的下标
func (f *Filter) calIdxAndBitOffset(offset int32) (int32, int32) {
	idx := offset >> 6             // offset / 64
	bitOffset := offset & (64 - 1) // offset % 64
	return idx, bitOffset
}

// Hash 将输入data转换为uint64的hash值
func Hash(data []byte) uint64 {
	return murmur3.Sum64(data)
}

```
```go
package bloom

import (
	"testing"
)

func TestBloomNew_Set_Exists(t *testing.T) {
	filter := NewFilter(1000, 10)

	isSetBefore := filter.check([]int32{0})
	if isSetBefore {
		t.Fatal("Bit should not be set")
	}

	filter.set([]int32{512})

	isSetAfter := filter.check([]int32{512})
	if !isSetAfter {
		t.Fatal("Bit should be set")
	}
}

func TestBloom(t *testing.T) {
	filter := NewFilter(50, 10)

	filter.Set("hello")
	filter.Set("world")
	exists := filter.Exists("hello")
	if !exists {
		t.Fatal("should be exists")
	}

	exists = filter.Exists("worrrrrld")
	if exists {
		t.Fatal("should be not exists")
	}
}

```
### 要点
代码中注释已经很详尽了，有几个要点额外指出：

1. 实际不需要使用多个 hash 函数，只需要使用同一个 hash 函数，向传入数据中撒盐，即可得到多个不同的 hash 值。
2. 得到的 hash 值实际是作为 bitmap 的下标，所以还需要根据 bitmap 范围取余，让下标落在初始化的 bitmap 范围内。
3. 使用 []uint64 作为 bitmap，需要注意下标从位到具体存放位置的转换，比如 bitmap 下标为 66，实际其实是 bitmap[1] << 2

![](https://cdn.nlark.com/yuque/0/2023/jpeg/2518584/1698885254335-15acc9aa-377a-4634-be5e-46badf3e5373.jpeg)
## Redis布隆过滤器的实现
### 存储结构
当多个服务需要共用布隆过滤器，就不能使用内存存储 bitmap，可以用Redis中的 bitmap 数据结构承载数据，Redis 中的 bitmap 底层使用的是动态字符串（SDS）实现，可以理解为 Golang 中的 Slice，支持自动扩容
Redis 提供了 `setbit`和 `getbit`命令可以用来处理二进制位数组
> setbit：为位数组指定偏移量上的二进制位设置值，偏移量从0开始计数，二进制位的值只能为 0 或 1，返回原位置值。
> getbit：获取指定偏移量上二进制的值。

```shell
> setbit foo 2 1
"0"
> setbit foo 3 1
"0"
> setbit foo 5 1
"0"
> getbit foo 0 
"0"
> getbit foo 3
"1"
> get foo
"4"       # 此时foo二进制位：00110100，对应 ASCII：4
```
### 代码实现
```go
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

```
```go
package bloom

import (
	"context"
	"github.com/longbridgeapp/assert"
	"testing"
)

func TestFilterSet(t *testing.T) {
	addr := "localhost:6379"
	pass := ""
	username := ""
	db := 0

	client := NewRedisConf(addr, pass, username, db)

	f := NewFilter(1000, 5, client)

	data := "test"
	key := "key1"

	err := f.Set(context.Background(), key, data)
	if err != nil {
		t.Errorf("Set error: %v", err)
	}

	val, err := f.Exists(context.Background(), key, data)
	if err != nil {
		t.Errorf("Get error: %v", err)
	} else {
		assert.Equal(t, true, val)
	}
}

func TestFilterExists(t *testing.T) {
	addr := "localhost:6379"
	pass := ""
	username := ""
	db := 0

	client := NewRedisConf(addr, pass, username, db)

	f := NewFilter(1000, 5, client)

	data := "test"
	data2 := "world!"
	key := "key1"

	err := f.Set(context.Background(), key, data)
	if err != nil {
		t.Errorf("Set error: %v", err)
	}

	err = f.Set(context.Background(), key, data2)
	if err != nil {
		t.Errorf("Set error: %v", err)
	}

	exists, err := f.Exists(context.Background(), key, data)
	if err != nil {
		t.Errorf("Exists error: %v", err)
	}

	if !exists {
		t.Error("Exists should be true")
	} else if err != nil {
		assert.Equal(t, true, exists)
	}

	exists, err = f.Exists(context.Background(), key, "hello,"+data2)
	if err != nil {
		t.Errorf("Exists error: %v", err)
	}

	if exists {
		t.Error("Exists should be false")
	} else {
		assert.Equal(t, false, exists)
	}
}

```
### 要点
代码中注释已经很详尽了，有几个要点额外指出：

1. 其实本质还是和前面的本地 bitmap 实现一样，只是 bitmap 使用了 Redis 的 bitmap，不再需要将手动转换下标，转而需要考虑需要如何和 Redis 进行交互
2. 可以利用 Redis 的 Script 保证命令的原子性
## go-zero 布隆过滤器的实现
> 基于 go-zero v1.5.5

看懂前面两轮本地+Redis布隆过滤器的粗糙实现，再来看看 go-zero 的工业级实现
### 对象定义
```go
// 固定使用 14 个 hash 函数
const maps = 14

// ErrTooLargeOffset 位偏移量过大
var	ErrTooLargeOffset = errors.New("too large offset")

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
```
### 位数组操作接口
go-zero中使用 lua 脚本保证原子性，其实前面我们已经用到了，这里再看一下
```go
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
```
自己实现时，在与 Redis 的交互上，有几点需要注意：

1. setScript 无返回参数，当不存在 key 时会返回 Redis.Nil，需要特殊处理
2. testScript 返回值为 true 或 false，C 语言无布尔类型，其实返回值为 1 or 0
3. setScript 和 testScript 中的 ARGV 需要传入 []string 格式，否则会报错无法解析

通过 Redis 操作 bitmap 接口如下：
```go
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
```
### k次散列计算出k个位点
go-zero 并没有使用 k 个散列函数计算同一份数据，而是使用同一个散列函数，每次计算位点时，在原数据上追加不同的下标混合计算，得到 k 个位点
```go
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
```
### 插入与查询
添加与查询实现就非常简单了，组合一下上面的函数就行。
```go
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
```
## 实战应用场景
在库存更新幂等设计中，我们当前使用的幂等方案为：

1. 创建幂等表，包括字段：op_trade_id，并为其建立唯一索引
2. 库存更新请求携带 op_trade_id
3. 库存更新前，向幂等表插入 op_trade_id（也可以先 get 判断，做完业务操作后再 insert）
   1. 如果成功插入，执行后续流程
   2. 如果插入失败报错唯一索引冲突，不再进行后续流程，直接返回

如果当前架构不满足性能要求，需要升级，可以考虑引入 Redis：

1. 库存请求携带 op_trade_id
2. Redis `set ex nx` 设置 op_trade_id：
   1. 如果成功设置，可以继续往下执行
   2. 如果未成功设置，不再进行后续流程，直接返回

过期时间的设置也有讲究，太短拦截不了重复请求，太长又太占内存了
> 本质：相当于使用 Redis 作为一个 map 去判断 op_trade_id 是否存在


为了节省内存，可以采用布隆过滤器，不过必须进行二次查库校验：

1. 库存请求携带 op_trade_id
2. 布隆过滤器校验：
   1. 如果 op_trade_id 不存在，可以继续往下执行
   2. 如果 op_trade_id 存在，通过查库二次校验，确定任务不存在才继续往下执行
3. 库内插入 op_trade_id
4. 定时任务，取表内一段时间内的 op_trade_id，新建布隆过滤器后 rename 之前的布隆过滤器

该方案的优点是既能确保使用到 Redis 作为缓存，也不至于占用太大内存，且能够完全保证幂等性


## 源码 & 参考 & 扩展
布隆过滤器源码地址：[https://github.com/callmePicacho/bloom](https://github.com/callmePicacho/bloom)

参考：

1. [扩展阅读 - 布隆过滤器 - 《go-zero v1.3 教程》 - 书栈网 · BookStack](https://www.bookstack.cn/read/go-zero-1.3-zh/bloom.md)
2. [go-zero基础组件-分布式布隆过滤器（Bloom Filter） - 掘金](https://juejin.cn/post/7026541493833695239)
3. [布隆过滤器技术原理及应用实战](https://zhuanlan.zhihu.com/p/648260944)

额外扩展：

1. [Counting Bloom Filter 的原理和实现-腾讯云开发者社区-腾讯云](https://cloud.tencent.com/developer/article/1136056)
2. [布谷鸟过滤器（Cuckoo Filter） - 泰阁尔 - 博客园](https://www.cnblogs.com/zhaodongge/p/15067657.html)
3. [一看就懂 详解redis的bitmap（面试加分项） - 掘金](https://juejin.cn/post/7074747080492711943)
4. [实战！聊聊幂等设计 - 掘金](https://juejin.cn/post/7049140742182141959?searchId=202311041533207C895CDAFF91957AAAEA#heading-11)
