package source

import (
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/zeromicro/go-zero/core/lang"
)

const (
	// TopWeight is the top weight that one entry might set.
	TopWeight = 100

	minReplicas = 100
	prime       = 16777619
)

type (
	// Func 定义hash函数
	Func func(data []byte) uint64

	// ConsistentHash 一致性哈希实现
	ConsistentHash struct {
		hashFunc Func                            // hash函数
		replicas int                             // 用于确定真实节点对应的虚拟节点数量，当AddWithWeight权重为100时，添加该真实节点，同时添加对应replicas数量的虚拟节点
		keys     []uint64                        // 虚拟节点列表
		ring     map[uint64][]any                // 虚拟节点到真实节点的映射，当存在冲突，多个真实节点追加到相同的 key
		nodes    map[string]lang.PlaceholderType // 真实节点的map，用于快速判断是否存在
		lock     sync.RWMutex                    // 读写锁
	}
)

// NewConsistentHash 创建默认hash环实例
func NewConsistentHash() *ConsistentHash {
	return NewCustomConsistentHash(minReplicas, Hash)
}

// NewCustomConsistentHash 自定义参数的一致性哈希实例
func NewCustomConsistentHash(replicas int, fn Func) *ConsistentHash {
	// 使用默认虚拟节点个数 100
	if replicas < minReplicas {
		replicas = minReplicas
	}

	// 使用默认哈希函数
	if fn == nil {
		fn = Hash
	}

	return &ConsistentHash{
		hashFunc: fn,
		replicas: replicas,
		ring:     make(map[uint64][]any),
		nodes:    make(map[string]lang.PlaceholderType),
	}
}

// Add 添加真实节点
func (h *ConsistentHash) Add(node any) {
	h.AddWithReplicas(node, h.replicas)
}

// AddWithReplicas adds the node with the number of replicas,
// replicas will be truncated to h.replicas if it's larger than h.replicas,
// the later call will overwrite the replicas of the former calls.
// AddWithReplicas 添加真实节点
func (h *ConsistentHash) AddWithReplicas(node any, replicas int) {
	// 支持重复添加
	// 先删除该真实节点
	h.Remove(node)

	// 每次添加真实节点，对应的虚拟节点个数不能超过该值
	if replicas > h.replicas {
		replicas = h.replicas
	}

	// 计算真实节点的key
	nodeRepr := repr(node)
	h.lock.Lock()
	defer h.lock.Unlock()
	// 将真实节点添加到nodes map中
	h.addNode(nodeRepr)

	for i := 0; i < replicas; i++ {
		// 计算虚拟节点的hash值
		hash := h.hashFunc([]byte(nodeRepr + strconv.Itoa(i)))
		// 添加虚拟节点
		h.keys = append(h.keys, hash)
		// 建立虚拟节点 -> 真实节点
		// 可能出现哈希冲突，使用链表法解决，追加到相同的切片中
		h.ring[hash] = append(h.ring[hash], node)
	}

	// 对虚拟节点hash值排序，方便快速找到虚拟节点
	sort.Slice(h.keys, func(i, j int) bool {
		return h.keys[i] < h.keys[j]
	})
}

// AddWithWeight 按百分比权重添加节点，权重越高，虚拟节点个数越多
func (h *ConsistentHash) AddWithWeight(node any, weight int) {
	// 最多添加h.replicas个的虚拟节点
	replicas := h.replicas * weight / TopWeight
	h.AddWithReplicas(node, replicas)
}

// Get 根据给定的 v 返回对应的节点
func (h *ConsistentHash) Get(v any) (any, bool) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	// 哈希环为空，返回 nil
	if len(h.ring) == 0 {
		return nil, false
	}

	// 针对 v 计算得到 hash 值
	hash := h.hashFunc([]byte(repr(v)))
	// 二分法返回第一个大于等于hash值的索引（相当于沿顺时针方向，找到第一个大于等于hash值的虚拟节点）
	index := sort.Search(len(h.keys), func(i int) bool {
		return h.keys[i] >= hash
	}) % len(h.keys)

	// index 虚拟节点的索引
	// h.keys[index] 找到对应虚拟节点的值
	// h.ring[h.keys[index]] 找到虚拟节点对应的真实节点切片
	nodes := h.ring[h.keys[index]]
	switch len(nodes) {
	case 0: // 如果虚拟节点没有对应的真实节点，返回空值
		return nil, false
	case 1: // 如果虚拟节点只有一个对应的真实节点，返回该真实节点
		return nodes[0], true
	default: // 如果虚拟节点有多个对应的真实节点，使用innerRepr(v)计算得到hash值，再使用该值在虚拟节点对应的切片中找到对应的节点
		innerIndex := h.hashFunc([]byte(innerRepr(v)))
		pos := int(innerIndex % uint64(len(nodes)))
		return nodes[pos], true
	}
}

// Remove 删除真实节点
func (h *ConsistentHash) Remove(node any) {
	// 返回node的字符串表示
	nodeRepr := repr(node)

	h.lock.Lock()
	defer h.lock.Unlock()

	// 真实节点不存在，直接返回
	if !h.containsNode(nodeRepr) {
		return
	}

	// 移除真实节点对应的虚拟节点
	for i := 0; i < h.replicas; i++ {
		// 计算虚拟节点的哈希值
		hash := h.hashFunc([]byte(nodeRepr + strconv.Itoa(i)))
		// 根据哈希值找到该虚拟节点的下标
		index := sort.Search(len(h.keys), func(i int) bool { return h.keys[i] >= hash })
		// 二次校验确实是这个虚拟节点，在虚拟节点切片上删除该虚拟节点
		if index < len(h.keys) && h.keys[index] == hash {
			// 使用index之后的元素（index+1）前移一位，覆盖index位置的元素
			h.keys = append(h.keys[:index], h.keys[index+1:]...)
		}
		// 删除虚拟节点到真实节点的映射
		h.removeRingNode(hash, nodeRepr)
	}

	h.removeNode(nodeRepr)
}

// 删除虚拟节点 -> 真实节点的映射关系
func (h *ConsistentHash) removeRingNode(hash uint64, nodeRepr string) {
	// 校验虚拟节点是否在哈希环中
	if nodes, ok := h.ring[hash]; ok {
		// 新建空的切片，保持原容量，等同于：make([]any, 0, len(nodes))
		newNodes := nodes[:0]
		// 遍历虚拟节点对应的真实节点列表，重新把非当前的真实节点加入到映射
		for _, x := range nodes {
			if repr(x) != nodeRepr {
				newNodes = append(newNodes, x)
			}
		}
		// 如果虚拟节点还有对应的真实节点，使用新真实节点列表作为映射的 value
		if len(newNodes) > 0 {
			h.ring[hash] = newNodes
		} else { // 否则直接删掉整个映射
			delete(h.ring, hash)
		}
	}
}

func (h *ConsistentHash) addNode(nodeRepr string) {
	h.nodes[nodeRepr] = lang.Placeholder
}

// 检查真实节点是否存储在hash环中
func (h *ConsistentHash) containsNode(nodeRepr string) bool {
	_, ok := h.nodes[nodeRepr]
	return ok
}

func (h *ConsistentHash) removeNode(nodeRepr string) {
	delete(h.nodes, nodeRepr)
}

// 在遇到哈希冲突时需要重新对key进行哈希计算
// 为了减少冲突的概率前面追加了一个质数 prime来减小冲突的概率
func innerRepr(node any) string {
	return fmt.Sprintf("%d:%v", prime, node)
}

// 返回 node 的字符串表示
func repr(node any) string {
	return lang.Repr(node)
}
