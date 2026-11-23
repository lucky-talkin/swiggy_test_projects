package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "order-api", "status": "ok"})
	})
	mux.HandleFunc("POST /v1/orders", notImplemented)
	mux.HandleFunc("GET /v1/orders/{id}", notImplemented)
	port := os.Getenv("PORT"); if port == "" { port = "8086" }
	log.Fatal((&http.Server{Addr: ":"+port, Handler:mux, ReadHeaderTimeout:5*time.Second}).ListenAndServe())
}
func notImplemented(w http.ResponseWriter, r *http.Request) { http.Error(w, "order persistence and Temporal client implementation pending", http.StatusNotImplemented) }
