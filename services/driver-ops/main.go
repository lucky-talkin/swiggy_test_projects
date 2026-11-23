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
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "driver-ops", "status": "ok"})
	})
	mux.HandleFunc("POST /v1/drivers/{driver_id}/availability", notImplemented)
	mux.HandleFunc("POST /v1/assignments", notImplemented)
	mux.HandleFunc("POST /v1/assignments/{order_id}/picked-up", notImplemented)
	mux.HandleFunc("POST /v1/assignments/{order_id}/delivered", notImplemented)
	port := os.Getenv("PORT"); if port == "" { port = "8085" }
	log.Fatal((&http.Server{Addr: ":"+port, Handler:mux, ReadHeaderTimeout:5*time.Second}).ListenAndServe())
}
func notImplemented(w http.ResponseWriter, r *http.Request) { http.Error(w, "PostgreSQL driver lifecycle implementation pending", http.StatusNotImplemented) }
