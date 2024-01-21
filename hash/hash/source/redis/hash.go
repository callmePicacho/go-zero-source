package redis

import "github.com/spaolacci/murmur3"

func Hash(data []byte) uint64 {
	return uint64(murmur3.Sum32(data))
}
