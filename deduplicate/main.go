package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type taskController struct {
	tasks    chan<- map[string]time.Duration
	requests chan<- chan<-[]byte
}

func (tc *taskController) Index (writer http.ResponseWriter, _ *http.Request) {
	response := make(chan []byte)
	tc.requests <- response

	if _, err := writer.Write(<-response); err != nil {
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
	tc.tasks <- tasks
}

type state bool

const (
	running state = true
	pending state = false
)

// Rob Pike style: Don't communicate by sharing memory; share memory by communicating
func processor(n int) (chan<- map[string]time.Duration, chan<- chan<-[]byte) {
	requests := make(chan chan<-[]byte)
	tasksInput := make(chan map[string]time.Duration, 1)

	go func() {
		taskStates := make(map[string]state)
		pendingTasks := make(map[string]time.Duration)
		completedTasks := make(chan string)
		runningCount := 0
		for {
			select {
			case tasks := <-tasksInput:
				for name, duration := range tasks {
					if runningCount == n {
						if _, exists := pendingTasks[name]; !exists {
							taskStates[name] = pending
							pendingTasks[name] = duration
						}
						continue
					}
					if taskStates[name] == running {
						continue
					}
					runningCount++
					taskStates[name] = running

					// TODO as an alternative fixed created number of goroutines or min-max pool if too many tasks
					go func(name string, duration time.Duration) {
						time.Sleep(duration)
						completedTasks <- name
					}(name, duration)
				}
			case response := <-requests:
				data, _ := json.Marshal(taskStates)
				response <- data
			case name := <-completedTasks:
				runningCount--
				delete(taskStates, name)
				delete(pendingTasks, name)

			RequeueTasksIfTheyExists:
				for {
					select {
					case tasks := <-tasksInput:
						for k, v := range tasks {
							if _, ok := pendingTasks[k]; !ok {
								pendingTasks[k] = v
							}
						}
					case tasksInput <- pendingTasks:
						break RequeueTasksIfTheyExists
					}
				}
			}
		}
	}()
	return tasksInput, requests
}

// duplicates omitted and not enqueued while we already have pending or executing task with the same name
func main() {
	var nFlag int
	flag.IntVar(&nFlag, "n", 10, "max number of concurrent executions")
	flag.Parse()

	tasks, requests := processor(nFlag)
	taskController := &taskController{tasks, requests}
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
