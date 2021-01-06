package main

import (
	"encoding/json"
	"log"
	"net/http"
)


type EnqueuerMarshaler interface {
	Enqueue(map[string]int)
	json.Marshaler
}

type TaskController struct {
	TaskManager EnqueuerMarshaler
}

func (tc *TaskController) Index (writer http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(writer).Encode(tc.TaskManager); err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), 500)
		return
	}
}

func (tc *TaskController) Create (writer http.ResponseWriter, request *http.Request) {
	tasks := make(map[string]int)
	if err := json.NewDecoder(request.Body).Decode(&tasks); err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), 500)
		return
	}
	tc.TaskManager.Enqueue(tasks)
}
