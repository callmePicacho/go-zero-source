package main

import (
	"fmt"
	"go-zero-source/executor/executor/source"
	"time"
)

type InsertTask struct {
	tasks   []any
	execute source.Execute
}

func newInsertTask(execute source.Execute) *InsertTask {
	return &InsertTask{
		execute: execute,
	}
}

// AddTask 将任务添加到容器中，并返回一个布尔值来指示是否需要在添加后刷新容器
func (i *InsertTask) AddTask(task any) bool {
	i.tasks = append(i.tasks, task)
	return len(i.tasks) >= 3
}

// Execute 刷新容器时处理收集的任务
func (i *InsertTask) Execute(tasks any) {
	vals := tasks.([]any)
	i.execute(vals)
}

// RemoveAll 移除并返回容器中的所有任务
func (i *InsertTask) RemoveAll() any {
	tasks := i.tasks
	i.tasks = nil
	return tasks
}

func main() {
	execute := func(tasks []any) {
		fmt.Println("执行了")
		for _, task := range tasks {
			fmt.Println(task)
		}
		time.Sleep(time.Second)
	}

	exec := source.NewPeriodicalExecutor(time.Second, newInsertTask(execute))

	for i := 10; i < 19; i++ {
		exec.Add(i)
	}

	exec.Flush()
}
