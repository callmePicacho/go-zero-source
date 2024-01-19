package source

import "sync"

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

func (s SliceHashRing) Lock() {
	//TODO implement me
	panic("implement me")
}

func (s SliceHashRing) UnLock() {
	//TODO implement me
	panic("implement me")
}

func (s SliceHashRing) AddNode(node string) {
	//TODO implement me
	panic("implement me")
}

func (s SliceHashRing) RemoveNode(node string) {
	//TODO implement me
	panic("implement me")
}

func (s SliceHashRing) ContainsNode(node string) bool {
	//TODO implement me
	panic("implement me")
}

func (s SliceHashRing) GetNode(virtualNode string) (string, bool) {
	//TODO implement me
	panic("implement me")
}

func (s SliceHashRing) AddVirtualNode(node, virtualNode string) {
	//TODO implement me
	panic("implement me")
}

func (s SliceHashRing) RemoveVirtualNode(node, virtualNode string) {
	//TODO implement me
	panic("implement me")
}

func (s SliceHashRing) GetVirtualNode(key string) (string, bool) {
	//TODO implement me
	panic("implement me")
}
