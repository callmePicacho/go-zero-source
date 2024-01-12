package main

import (
	"fmt"
	"go-zero-source/executor/go-zero/source"
	"math/rand"
	"time"
)

type InsertTask struct {
	tasks   []any
	execute executors.Execute
}

func newInsertTask(execute executors.Execute) *InsertTask {
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
	test2()
}

func test1() {
	exec := executors.NewBulkExecutor(func(tasks []any) {
		fmt.Println("到我执行了")
		for _, task := range tasks {
			fmt.Println(task)
		}
		time.Sleep(time.Second)
	}, executors.WithBulkTasks(3))

	for i := 10; i < 20; i++ {
		_ = exec.Add(i)
		time.Sleep(time.Millisecond * time.Duration(rand.Int31n(50)+80))
	}

	exec.Add(250)
	exec.Flush()
}

func test2() {
	execute := func(tasks []any) {
		fmt.Println("执行了")
		for _, task := range tasks {
			fmt.Println(task)
		}
		time.Sleep(time.Second)
	}

	exec := executors.NewPeriodicalExecutor(time.Second, newInsertTask(execute))

	defer exec.Wait()

	for i := 10; i < 19; i++ {
		exec.Add(i)
	}

	exec.Flush()
}

func test3() {
	exec := executors.NewChunkExecutor(func(tasks []any) {
		fmt.Println("到我执行了")
		for _, task := range tasks {
			fmt.Println(task)
		}
		time.Sleep(time.Second)
	}, executors.WithChunkBytes(30))
	defer exec.Wait()

	for i := 10; i < 20; i++ {
		_ = exec.Add(i, i)
		time.Sleep(time.Millisecond * time.Duration(rand.Int31n(50)+80))
	}

	_ = exec.Add(250, 250)
}
