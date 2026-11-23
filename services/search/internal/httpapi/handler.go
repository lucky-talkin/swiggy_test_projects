package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/lucky-talkin/swiggy_test_projects/services/search/internal/search"
)

type Handler struct {
	service *search.Service
	health  interface{ Ping(context.Context) error }
}

func NewHandler(service *search.Service, health interface{ Ping(context.Context) error }) *Handler {
	return &Handler{service: service, health: health}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("GET /v1/search", h.search)
	return requestID(mux)
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "search", "status": "ok"})
}

func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.health.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "search backend unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"service": "search", "status": "ready"})
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	request := search.SearchRequest{
		Query: query.Get("q"), Type: query.Get("type"), City: query.Get("city"),
		Cuisine: query.Get("cuisine"), RestaurantID: query.Get("restaurant_id"), Page: intParam(query.Get("page"), 1),
		PageSize: intParam(query.Get("page_size"), 20),
	}
	if value := query.Get("min_rating"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "min_rating must be a number")
			return
		}
		request.MinRating = parsed
	}
	if query.Has("lat") || query.Has("lon") {
		lat, latErr := strconv.ParseFloat(query.Get("lat"), 64)
		lon, lonErr := strconv.ParseFloat(query.Get("lon"), 64)
		if latErr != nil || lonErr != nil {
			writeError(w, http.StatusBadRequest, "lat and lon must be valid numbers and provided together")
			return
		}
		request.Latitude, request.Longitude = &lat, &lon
	}
	if value := query.Get("radius_km"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "radius_km must be a number")
			return
		}
		request.RadiusKM = parsed
	}
	result, err := h.service.Search(r.Context(), request)
	if err != nil {
		if errors.Is(err, search.ErrInvalidRequest) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "search backend request failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func intParam(value string, fallback int) int {
	if value == "" { return fallback }
	parsed, err := strconv.Atoi(value)
	if err != nil { return -1 }
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-ID") == "" {
			w.Header().Set("X-Request-ID", strconv.FormatInt(time.Now().UnixNano(), 36))
		} else {
			w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
		}
		next.ServeHTTP(w, r)
	})
}
