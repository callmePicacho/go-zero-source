package source

import (
	"go-zero-source/hash/hash/source/local"
	"strconv"
)

const (
	minReplicas = 100
)

type (
	HashFunc func(data []byte) uint64

	// ConsistentHash 一致性哈希实现
	ConsistentHash struct {
		hashRing HashRing // 哈希环
		hashFunc HashFunc // 哈希函数
		replicas int      // 添加真实节点时，结合权重，添加对应数量的虚拟节点
	}
)

func NewConsistentHash() *ConsistentHash {
	return NewCustomConsistentHash(local.NewSliceHashRing(), Hash, minReplicas)
}

// NewCustomConsistentHash 使用默认参数创建一致性哈希实例
func NewCustomConsistentHash(hashRing HashRing, hashFunc HashFunc, replicas int) *ConsistentHash {
	if hashRing == nil {
		hashRing = local.NewSliceHashRing() // 使用默认的hashRing
	}

	if hashFunc == nil {
		hashFunc = Hash
	}

	if replicas < minReplicas {
		replicas = minReplicas
	}

	return &ConsistentHash{
		hashRing: hashRing,
		hashFunc: hashFunc,
		replicas: replicas,
	}
}

// Add 添加真实节点
func (h *ConsistentHash) Add(node string) {
	// 支持重复添加
	// 先删除该真实节点
	h.Remove(node)

	// 加锁
	h.hashRing.Lock()
	defer h.hashRing.Unlock()

	for i := 0; i < h.replicas; i++ {
		// 计算虚拟节点的哈希值
		virtualNode := h.hashFunc([]byte(node + strconv.Itoa(i)))
		// 添加节点
		err := h.hashRing.AddNode(node, virtualNode, i)
		if err != nil {
			panic(err)
		}
	}
}

// Remove 删除真实节点
func (h *ConsistentHash) Remove(node string) {
	// 加锁
	h.hashRing.Lock()
	defer h.hashRing.Unlock()

	// 检查节点是否存在哈希环中，不存在直接返回
	if !h.hashRing.ContainsNode(node) {
		return
	}

	for i := 0; i < h.replicas; i++ {
		// 计算虚拟节点的哈希值
		virtualNode := h.hashFunc([]byte(node + strconv.Itoa(i)))
		// 删除节点
		err := h.hashRing.RemoveNode(node, virtualNode, i)
		if err != nil {
			panic(err)
		}
	}
}

// Get 查询节点，最终返回具体的真实节点
func (h *ConsistentHash) Get(key string) (string, bool) {
	// 加锁
	h.hashRing.Lock()
	defer h.hashRing.Unlock()

	// 计算key的哈希值
	hash := h.hashFunc([]byte(key))

	// 获取对应的真实节点
	return h.hashRing.GetNode(hash)
}
