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
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "restaurant-ops", "status": "ok"})
	})
	mux.HandleFunc("POST /v1/restaurant-orders/{order_id}/accept", notImplemented)
	mux.HandleFunc("POST /v1/restaurant-orders/{order_id}/reject", notImplemented)
	mux.HandleFunc("POST /v1/restaurant-orders/{order_id}/ready", notImplemented)
	port := os.Getenv("PORT"); if port == "" { port = "8084" }
	log.Fatal((&http.Server{Addr: ":"+port, Handler:mux, ReadHeaderTimeout:5*time.Second}).ListenAndServe())
}
func notImplemented(w http.ResponseWriter, r *http.Request) { http.Error(w, "PostgreSQL state transition implementation pending", http.StatusNotImplemented) }
