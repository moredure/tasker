package main

import (
	"encoding/json"
	"sync"
	"time"
)

// not so rob pike style (Don't communicate by sharing memory; share memory by communicating)
// duplicates not omitted, but enqueued by duplicated name
type EnqueuingTaskManager struct {
	lock *sync.RWMutex
	cond *sync.Cond
	tasks map[string][]int
	executing map[string]bool
	running int
	nFlag int
}

type TaskWithStateAndDuration struct {
	Name string
	Running bool
	Duration int
}

func (tc *EnqueuingTaskManager) MarshalJSON() ([]byte, error) {
	tc.lock.RLock()
	tasks := make([]*TaskWithStateAndDuration, 0)
	for name, durations := range tc.tasks {
		for i, duration := range durations {
			tasks = append(tasks, &TaskWithStateAndDuration{name, i == 0 && tc.executing[name], duration})
		}
	}
	tc.lock.RUnlock()
	return json.Marshal(tasks)
}

func (tc *EnqueuingTaskManager) Enqueue(tasks map[string]int) {
	tc.cond.L.Lock()
	for k, v := range tasks {
		tc.tasks[k] = append(tc.tasks[k], v)
	}
	tc.cond.Signal()
	tc.cond.L.Unlock()
}

func (t *EnqueuingTaskManager) Start() {
	for {
		t.cond.L.Lock()
		for len(t.tasks) == 0 || t.running == t.nFlag || len(t.tasks) == len(t.executing) { // all TaskWithStateAndDuration are executing
			t.cond.Wait()
		}
		name, duration := "", 0
		for k, v := range t.tasks {
			// allow exucution of only one TaskWithStateAndDuration with same name the rest of them will be enqueued and executed after it
			if t.executing[k] {
				continue
			}
			name, duration = k, v[0]
		}
		t.running++
		t.executing[name] = true
		t.cond.L.Unlock()

		// TODO as an alternative fixed created number of goroutines or min-max pool if too many tasks
		go func(name string, duration int) {
			time.Sleep(time.Duration(duration) * time.Millisecond)
			t.cond.L.Lock()
			delete(t.executing, name)
			if len(t.tasks[name]) == 1 {
				delete(t.tasks, name)
			} else {
				t.tasks[name] = t.tasks[name][1:]
			}
			t.running--
			t.cond.Signal()
			t.cond.L.Unlock()
		}(name, duration)
	}
}

func NewEnqueueTaskManager(nFlag int) *EnqueuingTaskManager {
	rwLock := new(sync.RWMutex)
	return &EnqueuingTaskManager{
		rwLock,
		sync.NewCond(rwLock),
		make(map[string][]int),
		make(map[string]bool),
		0,
		nFlag,
	}
}