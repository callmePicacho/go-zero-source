package main

import (
	"fmt"
	"go-zero-source/executor/executor/source/v1"
	"go-zero-source/executor/executor/source/v2"
	"time"
)

type InsertTask struct {
	tasks   []any
	execute v1.Execute
}

func newInsertTask(execute v1.Execute) *InsertTask {
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
	testv2()
}

func testv1() {
	execute := func(tasks []any) {
		fmt.Println("执行了")
		for _, task := range tasks {
			fmt.Println(task)
		}
		time.Sleep(time.Second)
	}

	exec := v1.NewPeriodicalExecutor(time.Second, newInsertTask(execute))

	for i := 10; i < 19; i++ {
		exec.Add(i)
	}

	// 可能导致任务未执行退出
	exec.Flush()
}

func testv2() {
	execute := func(tasks []any) {
		fmt.Println("执行了")
		for _, task := range tasks {
			fmt.Println(task)
		}
		time.Sleep(time.Second)
		fmt.Println("执行完成")
	}

	exec := v2.NewPeriodicalExecutor(time.Second, newInsertTask(execute))

	defer exec.Wait()

	for i := 10; i < 19; i++ {
		exec.Add(i)
	}

}
