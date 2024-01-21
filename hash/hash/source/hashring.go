package source

// HashRing 哈希环接口
type HashRing interface {
	Lock() error   // 加锁
	Unlock() error // 解锁

	AddNode(node string, virtualNode uint64, idx int) error    // 添加节点
	RemoveNode(node string, virtualNode uint64, idx int) error // 删除节点

	ContainsNode(node string) bool      // 检查节点是否存在
	GetNode(hash uint64) (string, bool) // 根据hash获取节点
}
