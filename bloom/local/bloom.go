package bloom

import "github.com/spaolacci/murmur3"

type Filter struct {
	bitmap []uint64
	k      int32 // hash 函数个数
	m      int32 // bitmap 的长度
}

// NewFilter 获取本地布隆过滤器
func NewFilter(m, k int32) *Filter {
	return &Filter{
		bitmap: make([]uint64, m/64+1),
		k:      k,
		m:      m,
	}
}

// Set 将元素添加到布隆过滤器中
func (f *Filter) Set(val string) {
	// 获取需要将 bitmap 置 1 的位下标
	locations := f.getLocations([]byte(val))
	f.set(locations)
}

// Exists 判定元素 val 是否存在
// - 当返回 false，该元素必定不存在
// - 当返回 true，该元素并非必定存在，可能不存在（假阳性）
func (f *Filter) Exists(val string) bool {
	// 获取该 val 对应 bitmap 中哪些位下标
	locations := f.getLocations([]byte(val))
	return f.check(locations)
}

// getLocations 使用 k 个 hash 函数，得到将要设置为 1 的位下标
func (f *Filter) getLocations(data []byte) []int32 {
	locations := make([]int32, f.k)
	for i := 0; int32(i) < f.k; i++ {
		// 使用不同序列号撒入 data，得到不同 hash
		hash := Hash(append(data, byte(i)))
		// 取余，确保得到的结果落在 bitmap 中
		locations[i] = int32(hash % uint64(f.m))
	}

	return locations
}

// set 将bitmap特定位置的值设置为1
func (f *Filter) set(offsets []int32) {
	for _, offset := range offsets {
		idx, bitOffset := f.calIdxAndBitOffset(offset)
		f.bitmap[idx] |= 1 << bitOffset
	}
}

// check 检查位下标对应的 bitmap，若存在值为 0，返回 false
func (f *Filter) check(offsets []int32) bool {
	for _, offset := range offsets {
		idx, bitOffset := f.calIdxAndBitOffset(offset)

		// 当某一位为 0，必定不存在
		if f.bitmap[idx]&(1<<bitOffset) == 0 {
			return false
		}
	}

	return true
}

// offset 是bitmap 的位下标，需要转换为 []uint64 数组的下标
func (f *Filter) calIdxAndBitOffset(offset int32) (int32, int32) {
	idx := offset >> 6             // offset / 64
	bitOffset := offset & (64 - 1) // offset % 64
	return idx, bitOffset
}

// Hash 将输入data转换为uint64的hash值
func Hash(data []byte) uint64 {
	return murmur3.Sum64(data)
}
