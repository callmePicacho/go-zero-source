package bloom

import (
	"context"
	"github.com/longbridgeapp/assert"
	"testing"
)

func TestFilterSet(t *testing.T) {
	addr := "localhost:6379"
	pass := ""
	username := ""
	db := 0

	client := NewRedisConf(addr, pass, username, db)

	f := NewFilter(1000, 5, client)

	data := "test"
	key := "key1"

	err := f.Set(context.Background(), key, data)
	if err != nil {
		t.Errorf("Set error: %v", err)
	}

	val, err := f.Exists(context.Background(), key, data)
	if err != nil {
		t.Errorf("Get error: %v", err)
	} else {
		assert.Equal(t, true, val)
	}
}

func TestFilterExists(t *testing.T) {
	addr := "localhost:6379"
	pass := ""
	username := ""
	db := 0

	client := NewRedisConf(addr, pass, username, db)

	f := NewFilter(1000, 5, client)

	data := "test"
	data2 := "world!"
	key := "key1"

	err := f.Set(context.Background(), key, data)
	if err != nil {
		t.Errorf("Set error: %v", err)
	}

	err = f.Set(context.Background(), key, data2)
	if err != nil {
		t.Errorf("Set error: %v", err)
	}

	exists, err := f.Exists(context.Background(), key, data)
	if err != nil {
		t.Errorf("Exists error: %v", err)
	}

	if !exists {
		t.Error("Exists should be true")
	} else if err != nil {
		assert.Equal(t, true, exists)
	}

	exists, err = f.Exists(context.Background(), key, "hello,"+data2)
	if err != nil {
		t.Errorf("Exists error: %v", err)
	}

	if exists {
		t.Error("Exists should be false")
	} else {
		assert.Equal(t, false, exists)
	}
}
