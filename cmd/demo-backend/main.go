package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	id := flag.String("id", "backend-1", "backend identifier")
	port := flag.Int("port", 9001, "listen port")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"backend_id":"%s","hostname":"%s","path":"%s","time":"%s"}`,
			*id,
			hostname(),
			r.URL.Path,
			time.Now().Format(time.RFC3339Nano),
		)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("demo backend %s listening on %s", *id, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
