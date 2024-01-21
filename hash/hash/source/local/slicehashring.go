package local

import (
	"math/rand"
	"sort"
	"sync"
)

// SliceHashRing 使用Slice实现HashRing接口
type SliceHashRing struct {
	keys  []uint64            // 虚拟节点列表
	ring  map[uint64][]string // 虚拟节点到真实节点的映射
	nodes map[string]struct{} // 真实节点map
	lock  sync.RWMutex
}

func NewSliceHashRing() *SliceHashRing {
	return &SliceHashRing{
		keys:  make([]uint64, 0),
		ring:  make(map[uint64][]string),
		nodes: make(map[string]struct{}),
	}
}

// Lock 加锁
func (s *SliceHashRing) Lock() error {
	s.lock.Lock()

	return nil
}

// Unlock 解锁
func (s *SliceHashRing) Unlock() error {
	s.lock.Unlock()

	return nil
}

// AddNode 添加真实节点、虚拟节点，建立虚拟节点到真实节点的映射
func (s *SliceHashRing) AddNode(node string, virtualNode uint64, _ int) error {
	// 添加真实节点
	s.nodes[node] = struct{}{}

	// 添加虚拟节点
	s.keys = append(s.keys, virtualNode)

	// 建立虚拟节点到真实节点的映射，当出现hash冲突，追加到切片中
	s.ring[virtualNode] = append(s.ring[virtualNode], node)

	// 为了保持有序性，每次添加虚拟节点都需要排序
	sort.Slice(s.keys, func(i, j int) bool { return s.keys[i] < s.keys[j] })

	return nil
}

// RemoveNode 删除真实节点、虚拟节点，删除虚拟节点到真实节点的映射
func (s *SliceHashRing) RemoveNode(node string, virtualNode uint64, _ int) error {
	// 删除真实节点
	delete(s.nodes, node)

	// 从存储虚拟节点的环上，找到虚拟节点
	idx := sort.Search(len(s.keys), func(i int) bool { return s.keys[i] >= virtualNode })

	// 删除该虚拟节点
	if idx < len(s.keys) && s.keys[idx] == virtualNode {
		// 使用idx后的元素，前移一位，覆盖掉s.key[idx]
		s.keys = append(s.keys[:idx], s.keys[idx+1:]...)
	}

	// 删除虚拟节点到真实节点的映射
	// 找到虚拟节点对应的真实节点列表
	if nodes, ok := s.ring[virtualNode]; ok {
		// 从真实节点列表中踢出该真实节点
		newNodes := make([]string, 0, len(nodes))
		for _, x := range nodes {
			if x != node {
				newNodes = append(newNodes, x)
			}
		}
		if len(newNodes) > 0 {
			s.ring[virtualNode] = newNodes
		} else {
			delete(s.ring, virtualNode)
		}
	}

	// 由于添加时节点有序，删除是用前移覆盖的方式删除的，不需要再排序
	return nil
}

// ContainsNode 判断真实节点是否存在
func (s *SliceHashRing) ContainsNode(node string) bool {
	_, ok := s.nodes[node]
	return ok
}

// GetNode 根据虚拟节点获取真实节点
func (s *SliceHashRing) GetNode(hash uint64) (string, bool) {
	// 哈希环为空，返回 nil
	if len(s.keys) == 0 {
		return "", false
	}

	// 找到第一个大于等于hash的虚拟节点（相当于顺时针）
	idx := sort.Search(len(s.keys), func(i int) bool { return s.keys[i] >= hash }) % len(s.keys)

	// 获取对应的真实节点列表 s.keys[idx]：虚拟节点
	nodes, ok := s.ring[s.keys[idx]]
	if !ok || len(nodes) == 0 {
		return "", false
	}

	if len(nodes) == 1 {
		return nodes[0], true
	}

	// 从列表中随机取出一个真实节点返回
	return nodes[rand.Intn(len(nodes))], true
}
