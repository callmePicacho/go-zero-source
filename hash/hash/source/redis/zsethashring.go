package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

const (
	defaultExpireSecond = 5 * time.Second
)

// ZSetHashRing 使用zset实现HashRing接口
type ZSetHashRing struct {
	// redis 存储哈希环的 key
	key    string
	client *redis.Client
}

func NewZSetHashRing(key, addr, passwd string) *ZSetHashRing {
	client := redis.NewClient(&redis.Options{Addr: addr, Password: passwd})

	// 删除 key
	_ = client.Del(context.Background(), key).Err()

	return &ZSetHashRing{
		key:    key,
		client: client,
	}
}

func (z *ZSetHashRing) getLockKey() string {
	return fmt.Sprintf("redis:consistent_hash:ring:lock:%s", z.key)
}

func (z *ZSetHashRing) Lock() error {
	// 使用 setnx 简单加锁
	return z.client.SetNX(context.Background(), z.getLockKey(), "", defaultExpireSecond).Err()
}

func (z *ZSetHashRing) Unlock() error {
	// 直接删锁
	return z.client.Del(context.Background(), z.getLockKey()).Err()
}

// AddNode 添加节点
// 相同 score 可能存在多个节点（发生hash冲突），如果冲突了需要合并放到一个元素中
func (z *ZSetHashRing) AddNode(node string, virtualNode uint64, idx int) error {
	score := strconv.FormatUint(virtualNode, 10)
	rawNodeKey := z.getRawNodeKey(node, idx)

	// 查询score位置是否存在member
	entities, err := z.client.ZRangeByScore(context.Background(), z.key, &redis.ZRangeBy{Min: score, Max: score}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("redis hash add fail, err:%w", err)
	}

	if len(entities) > 1 {
		return fmt.Errorf("invalid score entity len: %d", len(entities))
	}

	var members []string
	// 存在冲突，把已经存在的节点删掉
	if len(entities) == 1 {
		// 先反序列化为切片
		members = z.UnmarshalEntries(entities)

		// 已有该节点，返回
		for _, member := range members {
			if member == rawNodeKey {
				return nil
			}
		}

		// 删除当前节点
		err = z.client.ZRemRangeByScore(context.Background(), z.key, score, score).Err()
		if err != nil {
			return fmt.Errorf("redis ring add fail, err: %w", err)
		}
	}

	// 追加最新节点
	members = append(members, rawNodeKey)
	entitiesBytes := z.MarshalEntries(members)
	return z.client.ZAdd(context.Background(), z.key, &redis.Z{Score: float64(virtualNode), Member: entitiesBytes}).Err()
}

// RemoveNode 删除节点
// 相同 score 可能存在多个节点，此时需要先找到要删除的节点，再进行删除其中一个
func (z *ZSetHashRing) RemoveNode(node string, virtualNode uint64, idx int) error {
	score := strconv.FormatUint(virtualNode, 10)
	rawNodeKey := z.getRawNodeKey(node, idx)

	// 查询score位置是否存在member（寻找虚拟节点）
	entities, err := z.client.ZRangeByScore(context.Background(), z.key, &redis.ZRangeBy{Min: score, Max: score}).Result()
	// 没找到也算error
	if err != nil {
		return fmt.Errorf("redis hash remove fail, err:%w", err)
	}

	if len(entities) > 1 {
		return fmt.Errorf("invalid score entity len: %d", len(entities))
	}

	members := z.UnmarshalEntries(entities)

	// 判断虚拟节点中是否存在对应的真实节点
	index := -1
	for i, member := range members {
		if member == rawNodeKey {
			index = i
			break
		}
	}

	// 虚拟节点中不存在真实节点
	if index == -1 {
		return nil
	}

	// 从 member 中删除该真实节点
	members = append(members[:index], members[index+1:]...)

	// 删除整个元素
	err = z.client.ZRemRangeByScore(context.Background(), z.key, score, score).Err()
	if err != nil {
		return fmt.Errorf("redis ring add fail, err: %w", err)
	}

	// 如果已经空了，不必再添加
	if len(members) == 0 {
		return nil
	}

	// 新建元素
	entitiesBytes := z.MarshalEntries(members)
	return z.client.ZAdd(context.Background(), z.key, &redis.Z{Score: float64(virtualNode), Member: entitiesBytes}).Err()
}

// ContainsNode 检查节点是否存在
func (z *ZSetHashRing) ContainsNode(node string) bool {
	// 使用第一个去尝试看是否存在
	rawNodeKey := z.getRawNodeKey(node, 0)
	entitiesBytes := z.MarshalEntries([]string{rawNodeKey})
	err := z.client.ZScore(context.Background(), z.key, string(entitiesBytes)).Err()
	return !errors.Is(err, redis.Nil)
}

// GetNode 根据hash获取节点
// 1. 根据hash找到虚拟节点
// 2. 返回虚拟节点对应的一个真实节点
func (z *ZSetHashRing) GetNode(hash uint64) (string, bool) {
	// 找到虚拟节点
	members, ok := z.getVirtualNode(hash)
	if !ok {
		return "", false
	}

	if len(members) == 1 {
		return z.getRawNode(members[0]), true
	}

	// 如果存在多个，随机一个返回
	return members[rand.Intn(len(members))], true
}

// 获取虚拟节点对应的真实节点列表
// 先尝试 [hash, +inf) 区间内的第一个节点（顺时针），如果没找到，再找 (-inf, hash] 区间内的第一个节点（绕一个环回去找）
func (z *ZSetHashRing) getVirtualNode(hash uint64) ([]string, bool) {
	zrangeBy := &redis.ZRangeBy{
		Min:    strconv.FormatUint(hash, 10),
		Max:    "+inf",
		Offset: 0,
		Count:  1,
	}

	// 首先找 [hash, +inf] 区间内的第一个节点
	entries, err := z.client.ZRangeByScore(context.Background(), z.key, zrangeBy).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, false
	}

	if len(entries) != 0 {
		return z.UnmarshalEntries(entries), true
	}

	// 如果没找到，反过来找 [-inf, hash] 区间的第一个节点
	zrangeBy.Max = zrangeBy.Min
	zrangeBy.Min = "-inf"

	entries, err = z.client.ZRangeByScore(context.Background(), z.key, zrangeBy).Result()
	if err != nil && !errors.Is(err, redis.Nil) || len(entries) == 0 {
		return nil, false
	}

	return z.UnmarshalEntries(entries), true
}

func (z *ZSetHashRing) MarshalEntries(members []string) []byte {
	entriesBytes, _ := json.Marshal(members)

	return entriesBytes
}

func (z *ZSetHashRing) UnmarshalEntries(entries []string) []string {
	if len(entries) == 0 {
		return nil
	}

	var members []string
	_ = json.Unmarshal([]byte(entries[0]), &members)
	return members
}

// getRawNodeKey 为真实节点node添加后缀，区分不同节点
func (z *ZSetHashRing) getRawNodeKey(node string, idx int) string {
	return fmt.Sprintf("%s-%d", node, idx)
}

// getRawNode 根据真实节点key获取真实节点node
func (z *ZSetHashRing) getRawNode(rawNodeKey string) string {
	idx := strings.LastIndex(rawNodeKey, "-")
	return rawNodeKey[:idx]
}
