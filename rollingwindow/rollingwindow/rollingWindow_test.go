package rollingwindow

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

const duration = time.Millisecond * 500

func TestRollingWindowAdd(t *testing.T) {
	r := NewWindow(WithSize(3), WithInterval(duration))
	assert.Equal(t, int64(0), r.Reduce())
	r.Add(1)
	assert.Equal(t, int64(1), r.Reduce())
	elapse()
	r.Add(2)
	r.Add(3)
	assert.Equal(t, int64(6), r.Reduce())
	elapse()
	r.Add(4)
	r.Add(5)
	r.Add(6)
	assert.Equal(t, int64(21), r.Reduce())
	elapse()
	r.Add(7)
	assert.Equal(t, int64(27), r.Reduce())
}

func TestRollingWindowReduce(t *testing.T) {
	r := NewWindow(WithSize(4), WithInterval(duration))
	for i := 1; i <= 4; i++ {
		r.Add(int64(i * 10))
		elapse()
	}
	// 第一个桶过期
	assert.Equal(t, int64(90), r.Reduce())
}

func elapse() {
	time.Sleep(duration)
}
