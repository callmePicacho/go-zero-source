package source

import "strconv"

const (
	minReplicas = 100
)

type (
	HashFunc func(data []byte) string

	// ConsistentHash 一致性哈希实现
	ConsistentHash struct {
		hashRing HashRing // 哈希环
		hashFunc HashFunc // 哈希函数
		replicas int      // 添加真实节点时，结合权重，添加对应数量的虚拟节点
	}
)

// NewConsistentHash 使用默认参数创建一致性哈希实例
func NewConsistentHash(hashRing HashRing, hashFunc HashFunc, replicas int) *ConsistentHash {
	if hashRing == nil {
		hashRing = NewSliceHashRing() // 使用默认的hashRing
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
	defer h.hashRing.UnLock()

	for i := 0; i < h.replicas; i++ {
		// 计算虚拟节点的哈希值
		virtualNode := h.hashFunc([]byte(node + strconv.Itoa(i)))
		// 添加虚拟节点
		h.hashRing.AddVirtualNode(node, virtualNode)
	}

	// 添加真实节点
	h.hashRing.AddNode(node)
}

// Remove 删除真实节点
func (h *ConsistentHash) Remove(node string) {
	// 加锁
	h.hashRing.Lock()
	defer h.hashRing.UnLock()

	// 检查节点是否存在哈希环中，不存在直接返回
	if !h.hashRing.ContainsNode(node) {
		return
	}

	for i := 0; i < h.replicas; i++ {
		// 计算虚拟节点的哈希值
		virtualNode := h.hashFunc([]byte(node + strconv.Itoa(i)))
		// 删除虚拟节点
		h.hashRing.RemoveVirtualNode(node, virtualNode)
	}

	// 删除真实节点
	h.hashRing.RemoveNode(node)
}

// Get 查询节点，最终返回具体的真实节点
func (h *ConsistentHash) Get(key string) (string, bool) {
	// 加锁
	h.hashRing.Lock()
	defer h.hashRing.UnLock()

	// 计算key的哈希值
	hash := h.hashFunc([]byte(key))

	// 获取对应的虚拟节点
	virtualNode, ok := h.hashRing.GetVirtualNode(hash)
	if !ok {
		return "", false
	}

	// 根据虚拟节点获取对应的真实节点
	return h.hashRing.GetNode(virtualNode)
}
