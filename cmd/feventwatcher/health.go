
package main

import (
	"os"
	"fmt"
    "log"
	"time"
	"encoding/json"
	"net/http"
	"github.com/google/uuid"
)

var startTime time.Time

func uptime() time.Duration {
    return time.Since(startTime) / time.Millisecond
}

func init() {
    startTime = time.Now()
}

type HealthResponse struct {
	UUID string
	Now time.Time
	Runtime struct {
		Pid int
		Executable string
		Wd string
		Hostname string
		UptimeMillis time.Duration
	}
	Startup *Options
}

type healthHandler struct{
	options *Options
}

func Health(options *Options) {
	if options.Health.Port > 0 {
		s := &http.Server{
			Addr:           fmt.Sprintf(":%v", options.Health.Port),
			Handler:        &healthHandler{options: options},
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20,
		}
		log.Fatal(s.ListenAndServe())
	}
}

func (h *healthHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	responseBody := &HealthResponse{
		UUID:  uuid.New().String(),
		Now: time.Now(),
		Startup: h.options,
	}
	responseBody.Runtime.Pid = os.Getppid()
	responseBody.Runtime.Executable, _ = os.Executable()
	responseBody.Runtime.Wd, _ = os.Getwd()
	responseBody.Runtime.Hostname, _ = os.Hostname()
	responseBody.Runtime.UptimeMillis = uptime()

	data, err := json.Marshal(responseBody)
    if err != nil {
        http.Error(rw, err.Error(), http.StatusInternalServerError)
        return
    }
    rw.WriteHeader(200)
    rw.Header().Set("Content-Type", "application/json")
    rw.Write(data)
}