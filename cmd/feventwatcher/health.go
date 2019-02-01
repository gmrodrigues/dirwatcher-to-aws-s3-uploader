package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

var startTime time.Time

func uptime() time.Duration {
	return time.Since(startTime) / time.Millisecond
}

func init() {
	startTime = time.Now()
}

type HealthStatus struct {
	UUID    string
	Now     time.Time
	Runtime struct {
		Pid          int
		Executable   string
		Wd           string
		Hostname     string
		UptimeMillis time.Duration
	}
	Startup *Options
}

type healthHandler struct {
	options *Options
}

func NewHealthStatus(options *Options) *HealthStatus {
	status := &HealthStatus{
		UUID:    uuid.New().String(),
		Now:     time.Now(),
		Startup: options,
	}
	status.Runtime.Pid = os.Getppid()
	status.Runtime.Executable, _ = os.Executable()
	status.Runtime.Wd, _ = os.Getwd()
	status.Runtime.Hostname, _ = os.Hostname()
	status.Runtime.UptimeMillis = uptime()
	return status
}

func Health(options *Options) {
	if options.Health.Port > 0 {
		s := &http.Server{
			Addr:         fmt.Sprintf(":%v", options.Health.Port),
			Handler:      &healthHandler{options: options},
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		log.Fatal(s.ListenAndServe())
	}
}

func (h *healthHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	responseBody := NewHealthStatus(h.options)

	rw.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(rw).Encode(responseBody)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}
