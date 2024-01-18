package main

import (
	"fmt"
	"github.com/zeromicro/go-zero/core/hash"
	"math/rand"
)

func main() {
	fmt.Println("添加节点测试")
	addTest()

	fmt.Println("=========================================")

	fmt.Println("删除节点测试")

	removeTest()
}

// 添加节点测试
func addTest() {
	reqNum := 100000

	// 创建环
	dispatcher := hash.NewConsistentHash()
	nodes := []string{"localhost:8080", "localhost:8081", "localhost:8082", "localhost:8083", "localhost:8084"}

	// 添加真实节点
	for _, node := range nodes {
		dispatcher.Add(node)
	}

	batchGet(reqNum, dispatcher)

	addNode := "localhost:9090"
	fmt.Println("添加真实节点:", addNode)

	// 添加真实节点
	dispatcher.Add(addNode)

	batchGet(reqNum, dispatcher)
}

// 移除节点测试
func removeTest() {
	reqNum := 100000

	// 创建环
	dispatcher := hash.NewConsistentHash()
	nodes := []string{"localhost:8080", "localhost:8081", "localhost:8082", "localhost:8083", "localhost:8084"}

	// 添加真实节点
	for _, node := range nodes {
		dispatcher.Add(node)
	}

	batchGet(reqNum, dispatcher)

	fmt.Println("移除真实节点:", nodes[0])

	// 移除真实节点
	dispatcher.Remove(nodes[0])

	batchGet(reqNum, dispatcher)
}

func batchGet(reqNum int, dispatcher *hash.ConsistentHash) {
	nodeCount := make(map[string]int64)

	// 尝试请求 10w 次
	for i := 0; i < reqNum; i++ {
		node, ok := dispatcher.Get(rand.Int())
		if ok {
			nodeCount[node.(string)]++
		}
	}

	for node, count := range nodeCount {
		fmt.Printf("group %s:%d(%.02f%%)\n", node, count, float64(count)/float64(reqNum)*100)
	}
}

func test2() {
	// 按照长度hash，冲突特别大
	lenHashFunc := func(data []byte) uint64 {
		return uint64(len(data))
	}

	dispatcher := hash.NewCustomConsistentHash(100, lenHashFunc)
	nodes := []string{"localhost:808", "localhost:8081", "localhost:18082"}

	// 添加真实节点
	for weight, node := range nodes {
		dispatcher.AddWithWeight(node, weight+10)
	}

	nodeCount := make(map[string]int64)

	// 尝试请求 1w 次
	for i := 0; i < 10000; i++ {
		node, ok := dispatcher.Get(rand.Int())
		if ok {
			nodeCount[node.(string)]++
		}
	}

	fmt.Println("请求结果:", nodeCount)

}
