package main

import (
	"encoding/json"
	"sync"
	"time"
)

// alternative manager with deduplication based on goroutines
type SimpleTaskManager struct {
	sync.RWMutex
	semaphore chan struct{}
	states map[string]bool
}

func (tc *SimpleTaskManager) MarshalJSON() ([]byte, error) {
	tc.RLock()
	defer tc.RUnlock()
	return json.Marshal(tc.states)
}

func (tc *SimpleTaskManager) Enqueue(tasks map[string]int) {
	tc.Lock()
	defer tc.Unlock()
	for n, d := range tasks {
		if _, exists := tc.states[n]; exists {
			continue
		}
		tc.states[n] = false

		go func(name string, duration int) {
			tc.semaphore <- struct{}{}

			tc.Lock()
			tc.states[name] = true
			tc.Unlock()

			time.Sleep(time.Duration(duration) * time.Millisecond)

			tc.Lock()
			delete(tc.states, name)
			tc.Unlock()

			<-tc.semaphore
		}(n, d)
	}
}

func (tc *SimpleTaskManager) Start() {
	// to maintain compatibility with TaskManager interface
}

func NewSimpleTaskManager(nFlag int) *SimpleTaskManager {
	return &SimpleTaskManager{
		semaphore: make(chan struct{}, nFlag),
	}
}
