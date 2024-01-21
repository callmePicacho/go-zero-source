package source

import (
	"github.com/spaolacci/murmur3"
)

func Hash(data []byte) uint64 {
	return murmur3.Sum64(data)
}
