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
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "user-auth", "status": "ok"})
	})
	mux.HandleFunc("POST /v1/auth/register", notImplemented)
	mux.HandleFunc("POST /v1/auth/login", notImplemented)
	port := env("PORT", "8081")
	s := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("user-auth listening on :%s", port)
	log.Fatal(s.ListenAndServe())
}
func notImplemented(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "endpoint scaffolded; persistence/auth implementation pending", http.StatusNotImplemented)
}
func env(k, fallback string) string { if v := os.Getenv(k); v != "" { return v }; return fallback }
