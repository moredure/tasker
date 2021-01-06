package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type taskController struct {
	// communicate by shared memory
	lock *sync.RWMutex
	cond *sync.Cond
	data map[string][]time.Duration
	executing map[string]bool
}

type Task struct {
	Name string
	Running bool
	Duration time.Duration
}

func (tc *taskController) Index (writer http.ResponseWriter, _ *http.Request) {
	tc.lock.RLock()
	tasks := make([]*Task, 0)
	for name, durations := range tc.data {
		for i, duration := range durations {
			tasks = append(tasks, &Task{name, i == 0 && tc.executing[name], duration})
		}
	}
	tc.lock.RUnlock()

	data, _ := json.Marshal(tasks)
	if _, err := writer.Write(data); err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), 500)
		return
	}
}

func (tc *taskController) Create (writer http.ResponseWriter, request *http.Request) {
	tasks := make(map[string]time.Duration)
	if err := json.NewDecoder(request.Body).Decode(&tasks); err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), 500)
		return
	}

	tc.cond.L.Lock()
	for k, v := range tasks {
		tc.data[k] = append(tc.data[k], v)
	}
	tc.cond.Signal()
	tc.cond.L.Unlock()
}

// not so rob pike style (Don't communicate by sharing memory; share memory by communicating)
// duplicated not omitted but enqueued by duplicated name
func main() {
	var nFlag int
	flag.IntVar(&nFlag, "n", 10, "max number of concurrent executions")
	flag.Parse()

	rwLock := new(sync.RWMutex)
	cond := sync.NewCond(rwLock)
	tasks := make(map[string][]time.Duration)
	executing := make(map[string]bool)
	running := 0

	go func() {
		for {
			cond.L.Lock()
			for len(tasks) == 0 || running == nFlag || len(tasks) == len(executing) { // all task are executing
				cond.Wait()
			}
			name, duration := "", time.Duration(0)
			for k, v := range tasks {
				// allow exucution of only one task with same name the rest of them will be enqueued and executed after it
				if executing[k] {
					continue
				}
				name, duration = k, v[0]
			}
			running++
			executing[name] = true
			cond.L.Unlock()

			// TODO as an alternative fixed created number of goroutines or min-max pool if too many tasks
			go func(name string, duration time.Duration) {
				time.Sleep(duration)
				cond.L.Lock()
				delete(executing, name)
				if len(tasks[name]) == 1 {
					delete(tasks, name)
				} else {
					tasks[name] = tasks[name][1:]
				}
				running--
				cond.Signal()
				cond.L.Unlock()
			}(name, duration)
		}
	}()

	taskController := &taskController{rwLock, cond, tasks, executing}
	m := http.NewServeMux()
	m.Handle("/tasks", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			taskController.Index(writer, request)
		case http.MethodPost:
			taskController.Create(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}))
	s := http.Server{
		Addr:              ":8080",
		Handler: m,
	}
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-exit
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}
