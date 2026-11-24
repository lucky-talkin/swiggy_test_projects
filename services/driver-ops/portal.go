package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type driverAssignmentView struct {
	OrderID          string          `json:"order_id"`
	DriverID         string          `json:"driver_id"`
	RestaurantID     string          `json:"restaurant_id"`
	CustomerID       string          `json:"customer_id"`
	Items            json.RawMessage `json:"items"`
	DeliveryAddress  json.RawMessage `json:"delivery_address"`
	TotalAmountMinor int64           `json:"total_amount_minor"`
	Status           string          `json:"status"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func (a *app) registerPortalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/portal/drivers/{driver_id}", a.portalDriver)
	mux.HandleFunc("GET /v1/portal/drivers/{driver_id}/assignments", a.portalAssignments)
	mux.HandleFunc("GET /v1/portal/drivers/{driver_id}/events", a.portalEvents)
	mux.HandleFunc("POST /v1/portal/drivers/{driver_id}/availability", a.portalAvailability)
	mux.HandleFunc("POST /v1/portal/drivers/{driver_id}/assignments/{order_id}/picked-up", func(w http.ResponseWriter, r *http.Request) {
		a.portalTransition(w, r, "PICKED_UP", "driver_picked_up")
	})
	mux.HandleFunc("POST /v1/portal/drivers/{driver_id}/assignments/{order_id}/delivered", func(w http.ResponseWriter, r *http.Request) {
		a.portalTransition(w, r, "DELIVERED", "delivery_completed")
	})
}

func (a *app) portalAuthorized(r *http.Request) bool {
	return r.Header.Get("X-Portal-Token") == env("PORTAL_ACCESS_TOKEN", "dev-portal-token")
}

func (a *app) portalDriver(w http.ResponseWriter, r *http.Request) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	id := r.PathValue("driver_id")
	var status string
	err := a.db.QueryRow(r.Context(), "SELECT status FROM drivers WHERE driver_id=$1", id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "DRIVER_NOT_FOUND", "driver does not exist")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	respond(w, 200, map[string]string{"driver_id": id, "status": status})
}

func (a *app) portalAvailability(w http.ResponseWriter, r *http.Request) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	r.Header.Set("X-Service-Token", a.token)
	r.SetPathValue("driver_id", r.PathValue("driver_id"))
	a.setAvailability(w, r)
}

func (a *app) portalAssignments(w http.ResponseWriter, r *http.Request) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	assignments, err := a.listDriverAssignments(r.Context(), r.PathValue("driver_id"))
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	respond(w, 200, map[string]any{"assignments": assignments})
}

func (a *app) listDriverAssignments(ctx context.Context, driverID string) ([]driverAssignmentView, error) {
	rows, err := a.db.Query(ctx, `
		SELECT order_id, driver_id, restaurant_id, customer_id, items, delivery_address,
		       total_amount_minor, status, created_at, updated_at
		FROM driver_assignments
		WHERE driver_id=$1
		ORDER BY updated_at DESC
		LIMIT 100`, driverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assignments := make([]driverAssignmentView, 0)
	for rows.Next() {
		var item driverAssignmentView
		var items, address []byte
		if err := rows.Scan(&item.OrderID, &item.DriverID, &item.RestaurantID, &item.CustomerID,
			&items, &address, &item.TotalAmountMinor, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Items = json.RawMessage(items)
		item.DeliveryAddress = json.RawMessage(address)
		assignments = append(assignments, item)
	}
	return assignments, rows.Err()
}

func (a *app) portalTransition(w http.ResponseWriter, r *http.Request, target, signal string) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	var owner string
	err := a.db.QueryRow(r.Context(), "SELECT driver_id FROM driver_assignments WHERE order_id=$1", r.PathValue("order_id")).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || owner != r.PathValue("driver_id") {
		fail(w, 404, "ASSIGNMENT_NOT_FOUND", "assignment does not belong to this driver")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	r.Header.Set("X-Service-Token", a.token)
	a.transition(w, r, target, signal)
}

func (a *app) portalEvents(w http.ResponseWriter, r *http.Request) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "STREAM_UNAVAILABLE", "streaming is not supported")
		return
	}
	id := r.PathValue("driver_id")
	last, err := a.latestDriverUpdate(r.Context(), id)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	writePortalEvent(w, "refresh", map[string]string{"reason": "connected"})
	flusher.Flush()

	poll := time.NewTicker(time.Second)
	defer poll.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			updated, queryErr := a.latestDriverUpdate(r.Context(), id)
			if queryErr != nil {
				fmt.Fprintf(w, "event: service-error\ndata: %q\n\n", queryErr.Error())
				flusher.Flush()
				continue
			}
			if updated.After(last) {
				last = updated
				writePortalEvent(w, "refresh", map[string]string{"updated_at": updated.UTC().Format(time.RFC3339Nano)})
				flusher.Flush()
			}
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func (a *app) latestDriverUpdate(ctx context.Context, driverID string) (time.Time, error) {
	var updated time.Time
	err := a.db.QueryRow(ctx, `
		SELECT GREATEST(
			COALESCE((SELECT MAX(updated_at) FROM driver_assignments WHERE driver_id=$1), 'epoch'::timestamptz),
			COALESCE((SELECT updated_at FROM drivers WHERE driver_id=$1), 'epoch'::timestamptz)
		)`, driverID).Scan(&updated)
	return updated, err
}
