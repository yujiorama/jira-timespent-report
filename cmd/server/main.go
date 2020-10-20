package main

import (
	"bitbucket.org/yujiorama/jira-timespent-report/jira"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

const (
	DefaultHost = "localhost"
	DefaultPort = "8080"
)

func main() {

	log.Println("start")
	jira.SetFlags()

	serverHost := DefaultHost
	if v, ok := os.LookupEnv("SERVER_HOST"); ok {
		if len(v) > 0 {
			serverHost = v
		}
	}
	serverPort := DefaultPort
	if v, ok := os.LookupEnv("SERVER_PORT"); ok {
		if len(v) > 0 {
			serverPort = v
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/report", reportHandler)

	s := &http.Server{
		Addr:    serverHost + ":" + serverPort,
		Handler: mux,
	}

	err := s.ListenAndServe()
	if err != nil {
		log.Fatal("Error Starting the HTTP Server : ", err)
		return
	}

	log.Println("end")
}

type errorResponse struct {
	Message []string `json:"message"`
}

func handleError(responseBody *errorResponse, w http.ResponseWriter) {

	body, err := json.Marshal(&responseBody)
	if err != nil {
		log.Println(err)
	}

	h := w.Header()
	h.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_, err = w.Write(body)
	if err != nil {
		log.Println(err)
	}
}

func reportHandler(w http.ResponseWriter, r *http.Request) {

	issues, worklogs, searchErrors := jira.Search()
	if searchErrors != nil && len(searchErrors) > 0 {
		message := make([]string, 0, 10)
		for _, err := range searchErrors {
			log.Printf("%v\n", err)
			message = append(message, fmt.Sprintf("%v", err))
		}

		responseBody := &errorResponse{Message: message}

		handleError(responseBody, w)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/csv")
	reportErrors := jira.Report(w, issues, worklogs)
	if reportErrors != nil {
		for _, err := range reportErrors {
			log.Printf("%v\n", err)
		}
	}
}
