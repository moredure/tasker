package main

import (
	"encoding/json"
	"time"
)


// rob pike style goroutine-actor (Don't communicate by sharing memory; share memory by communicating)
// duplicates  omitted, and not enqueued
type DeduplicatedTaskManager struct {
	tasksInput chan map[string]int
	requests chan chan<-[]*TaskWithState
	n int
}

type TaskWithState struct {
	Name string
	Running bool
}

func NewDeduplicatedTaskManager(n int) *DeduplicatedTaskManager {
	return &DeduplicatedTaskManager{
		tasksInput: make(chan map[string]int, 1),
		requests:   make(chan chan<-[]*TaskWithState),
		n:          n,
	}
}

func (d *DeduplicatedTaskManager) Enqueue(tasks map[string]int) {
	d.tasksInput <- tasks
}

func (d *DeduplicatedTaskManager) MarshalJSON() ([]byte, error) {
	response := make(chan []*TaskWithState)
	d.requests <- response
	return json.Marshal(<-response)
}

func (d *DeduplicatedTaskManager) Start() {
	taskStates := make(map[string]bool)
	pendingTasks := make(map[string]int)
	completedTasks := make(chan string)
	runningCount := 0

	for {
		select {
		case tasks := <-d.tasksInput:
			for name, duration := range tasks {
				if runningCount == d.n {
					if _, exists := pendingTasks[name]; !exists {
						taskStates[name] = false
						pendingTasks[name] = duration
					}
					continue
				}
				if taskStates[name] {
					continue
				}
				runningCount++
				taskStates[name] = true

				// TODO as an alternative fixed created number of goroutines or min-max pool if too many tasks
				go func(name string, duration int) {
					time.Sleep(time.Duration(duration) * time.Millisecond)
					completedTasks <- name
				}(name, duration)
			}
		case response := <-d.requests:
			tasks := make([]*TaskWithState, len(taskStates))
			for name, state := range taskStates {
				tasks = append(tasks, &TaskWithState{Name: name, Running: state})
			}
			response <- tasks
		case name := <-completedTasks:
			runningCount--
			delete(taskStates, name)
			delete(pendingTasks, name)

		RequeueTasksIfTheyExists:
			for {
				select {
				case tasks := <-d.tasksInput:
					for k, v := range tasks {
						if _, ok := pendingTasks[k]; !ok {
							pendingTasks[k] = v
						}
					}
				case d.tasksInput <- pendingTasks:
					break RequeueTasksIfTheyExists
				}
			}
		}
	}
}
