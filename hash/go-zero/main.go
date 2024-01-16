package main

import (
	"fmt"
	"github.com/zeromicro/go-zero/core/hash"
	"math/rand"
)

func main() {
	dispatcher := hash.NewConsistentHash()
	nodes := []string{"localhost:8080", "localhost:8081", "localhost:8082"}

	for weight, node := range nodes {
		dispatcher.AddWithWeight(node, weight+10)
	}

	nodeCount := make(map[string]int64)

	for i := 0; i < 10000; i++ {
		node, ok := dispatcher.Get(rand.Int())
		if ok {
			nodeCount[node.(string)]++
		}
	}

	fmt.Println("请求结果:", nodeCount)
}
