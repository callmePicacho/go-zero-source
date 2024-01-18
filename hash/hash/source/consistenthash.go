package source

import "sync"

const (
	minReplicas = 100
)

type (
	HashFunc func(data []byte) uint64

	// HashRing 哈希环接口
	HashRing interface {
		// AddNode 添加节点到哈希环
		AddNode(node any)
		// RemoveNode 从哈希环中删除节点
		RemoveNode(node any)
	}

	// ConsistentHash 一致性哈希实现
	ConsistentHash struct {
		hashRing HashRing // 哈希环
		hashFunc HashFunc // 哈希函数
		replicas int      // 添加真实节点时，结合权重，添加对应数量的虚拟节点
	}
)

// NewConsistentHash 使用默认参数创建一致性哈希实例
func NewConsistentHash() *ConsistentHash {
	//return NewCustomConsistentHash(minReplicas, repr)
	panic(nil)
}

func NewCustomConsistentHash(hashRing HashRing, hashFunc HashFunc, replicas int) *ConsistentHash {
	panic(nil)
}

// SliceHashRing 使用Slice实现HashRing接口
type SliceHashRing struct {
	keys  []uint64            // 虚拟节点列表
	ring  map[uint64][]any    // 虚拟节点到真实节点的映射
	nodes map[string]struct{} // 真实节点map
	lock  sync.RWMutex
}

func NewSliceHashRing() *SliceHashRing {
	return &SliceHashRing{
		keys:  make([]uint64, 0),
		ring:  make(map[uint64][]any),
		nodes: make(map[string]struct{}),
	}
}
