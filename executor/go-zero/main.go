package main

import (
	"fmt"
	"github.com/zeromicro/go-zero/core/executors"
	"math/rand"
	"time"
)

type InsertTask struct {
	tasks   []int
	execute func(tasks any)
}

func newInsertTask(execute func(tasks any)) *InsertTask {
	return &InsertTask{
		execute: execute,
	}
}

// AddTask 将任务添加到容器中，并返回一个布尔值来指示是否需要在添加后刷新容器
func (i *InsertTask) AddTask(task any) bool {
	i.tasks = append(i.tasks, task.(int))
	return len(i.tasks) >= 10
}

// Execute 刷新容器时处理收集的任务
func (i *InsertTask) Execute(tasks any) {
	if i.execute != nil {
		i.execute(tasks)
	} else {
		fmt.Println("sleep 1s")
		time.Sleep(time.Second)
	}
}

// RemoveAll 移除并返回容器中的所有任务
func (i *InsertTask) RemoveAll() any {
	tasks := i.tasks
	i.tasks = nil
	return tasks
}

// go-zero 中executors充当任务池，做多任务缓冲
func main() {
	exec := executors.NewPeriodicalExecutor(time.Millisecond*100, newInsertTask(func(tasks any) {
		fmt.Println("到我执行了")
		for _, task := range tasks.([]int) {
			fmt.Println(task)
		}
		time.Sleep(time.Second)
	}))

	for i := 10; i < 20; i++ {
		exec.Add(i)
		time.Sleep(time.Millisecond * time.Duration(rand.Int31n(50)+80))
	}

	exec.Add(250)
	exec.Flush()
}
