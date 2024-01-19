package source

import (
	"github.com/spaolacci/murmur3"
	"strconv"
)

func Hash(data []byte) string {
	return strconv.FormatUint(murmur3.Sum64(data), 64)
}
