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

type TaskManager interface {
	json.Marshaler
	Start()
	Enqueue(map[string]int)
}


func main() {
	var nFlag int
	var deduplicate bool

	flag.IntVar(&nFlag, "n", 10, "max number of concurrent executions")
	flag.BoolVar(&deduplicate, "deduplicate", true, "deduplicate or duplicated engine")
	flag.Parse()

	var tm TaskManager

	if deduplicate {
		tm = NewDeduplicatedTaskManager(nFlag)
	} else {
		tm = NewEnqueuingTaskManager(nFlag)
	}

	taskController := &TaskController{tm}

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
	go tm.Start()
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
