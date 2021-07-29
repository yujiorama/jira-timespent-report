package web

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bitbucket.org/yujiorama/jira-timespent-report/jira"
)

const (
	DefaultHost = "localhost"
	DefaultPort = 8080
)

var (
	serverEnable bool
	host         string
	port         int
)

func init() {
	flag.BoolVar(&serverEnable, "server", false, "server mode")
	flag.StringVar(&host, "host", DefaultHost, "request host")
	flag.IntVar(&port, "port", DefaultPort, "request port")
}

func CanDo() bool {

	return serverEnable
}

func Do() {
	log.Println("start")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/", reportHandler)

	server := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", host, port),
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server ListenAndServe: %v", err)
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(
		signalChan,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGQUIT,
	)
	<-signalChan

	gracefulCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(gracefulCtx); err != nil {
		log.Printf("shutdown error: %v\n", err)
		defer os.Exit(1)
		return
	}

	log.Printf("end\n")

	defer os.Exit(0)
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

	jira.SetQueryParams(r.URL.Query())

	issues, worklogs, searchErrors := jira.Search()
	if len(searchErrors) > 0 {
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
	for _, err := range reportErrors {
		log.Printf("%v\n", err)
	}
}
