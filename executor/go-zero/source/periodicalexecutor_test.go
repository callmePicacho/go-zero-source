package executors

import (
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

func TestPeriodicalExecutor_Deadlock(t *testing.T) {
	executor := NewBulkExecutor(func(tasks []any) {
	}, WithBulkTasks(1), WithBulkInterval(time.Millisecond))
	for i := 0; i < 1e5; i++ {
		executor.Add(1)
	}
}

func TestPeriodicalExecutor_WaitFast(t *testing.T) {
	const total = 3
	var cnt int
	var lock sync.Mutex
	executor := NewBulkExecutor(func(tasks []any) {
		defer func() {
			cnt++
		}()
		lock.Lock()
		defer lock.Unlock()
		time.Sleep(10 * time.Millisecond)
	}, WithBulkTasks(1), WithBulkInterval(10*time.Millisecond))
	for i := 0; i < total; i++ {
		executor.Add(2)
	}
	executor.Flush()
	executor.Wait()
	assert.Equal(t, total, cnt)
}
