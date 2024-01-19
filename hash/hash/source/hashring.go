package source

// HashRing 哈希环接口
type HashRing interface {
	Lock()   // 加锁
	UnLock() // 解锁

	AddNode(node string)                       // 添加真实节点
	RemoveNode(node string)                    // 删除真实节点
	ContainsNode(node string) bool             // 检查真实节点是否存在
	GetNode(virtualNode string) (string, bool) // 根据虚拟节点获取真实节点

	AddVirtualNode(node, virtualNode string)    // 添加虚拟节点，建立虚拟节点和真实节点的映射
	RemoveVirtualNode(node, virtualNode string) // 删除虚拟节点，删除虚拟节点和真实节点的映射
	GetVirtualNode(key string) (string, bool)   // 根据 key 获取虚拟节点
}
