package main

import (
	"fmt"
	"github.com/zeromicro/go-zero/core/hash"
	"math/rand"
)

func main() {
	test2()
}

func test1() {
	// 创建环
	dispatcher := hash.NewConsistentHash()
	nodes := []string{"localhost:8080", "localhost:8081", "localhost:8082"}

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

	// 移除真实节点
	dispatcher.Remove(nodes[0])

	nodeCount = make(map[string]int64)

	// 尝试请求 1w 次
	for i := 0; i < 10000; i++ {
		node, ok := dispatcher.Get(rand.Int())
		if ok {
			nodeCount[node.(string)]++
		}
	}

	fmt.Println("请求结果:", nodeCount)
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
