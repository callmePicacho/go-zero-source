package bloom

import (
	"testing"
)

func TestBloomNew_Set_Exists(t *testing.T) {
	filter := NewFilter(1000, 10)

	isSetBefore := filter.check([]int32{0})
	if isSetBefore {
		t.Fatal("Bit should not be set")
	}

	filter.set([]int32{512})

	isSetAfter := filter.check([]int32{512})
	if !isSetAfter {
		t.Fatal("Bit should be set")
	}
}

func TestBloom(t *testing.T) {
	filter := NewFilter(50, 10)

	filter.Set("hello")
	filter.Set("world")
	exists := filter.Exists("hello")
	if !exists {
		t.Fatal("should be exists")
	}

	exists = filter.Exists("worrrrrld")
	if exists {
		t.Fatal("should be not exists")
	}
}
