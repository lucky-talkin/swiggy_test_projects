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

type restaurantOrderView struct {
	OrderID          string          `json:"order_id"`
	RestaurantID     string          `json:"restaurant_id"`
	CustomerID       string          `json:"customer_id"`
	Items            json.RawMessage `json:"items"`
	DeliveryAddress  json.RawMessage `json:"delivery_address"`
	TotalAmountMinor int64           `json:"total_amount_minor"`
	Status           string          `json:"status"`
	TicketCreated    bool            `json:"ticket_created"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func (a *app) registerPortalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/portal/restaurants/{restaurant_id}", a.portalRestaurant)
	mux.HandleFunc("GET /v1/portal/restaurants/{restaurant_id}/orders", a.portalOrders)
	mux.HandleFunc("GET /v1/portal/restaurants/{restaurant_id}/events", a.portalEvents)
	mux.HandleFunc("POST /v1/portal/restaurants/{restaurant_id}/availability", a.portalAvailability)
	mux.HandleFunc("POST /v1/portal/restaurants/{restaurant_id}/orders/{order_id}/accept", func(w http.ResponseWriter, r *http.Request) {
		a.portalTransition(w, r, "ACCEPTED", "restaurant_decision", map[string]string{"decision": "ACCEPTED"})
	})
	mux.HandleFunc("POST /v1/portal/restaurants/{restaurant_id}/orders/{order_id}/reject", func(w http.ResponseWriter, r *http.Request) {
		a.portalTransition(w, r, "REJECTED", "restaurant_decision", map[string]string{"decision": "REJECTED"})
	})
	mux.HandleFunc("POST /v1/portal/restaurants/{restaurant_id}/orders/{order_id}/ready", func(w http.ResponseWriter, r *http.Request) {
		a.portalTransition(w, r, "READY", "restaurant_ready", map[string]string{"state": "READY"})
	})
}

func (a *app) portalAuthorized(r *http.Request) bool {
	return r.Header.Get("X-Portal-Token") == env("PORTAL_ACCESS_TOKEN", "dev-portal-token") ||
		r.URL.Query().Get("token") == env("PORTAL_ACCESS_TOKEN", "dev-portal-token")
}

func (a *app) portalRestaurant(w http.ResponseWriter, r *http.Request) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	id := r.PathValue("restaurant_id")
	var accepting bool
	err := a.db.QueryRow(r.Context(), "SELECT accepting_orders FROM restaurant_settings WHERE restaurant_id=$1", id).Scan(&accepting)
	if errors.Is(err, pgx.ErrNoRows) {
		accepting = true
	} else if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	respond(w, 200, map[string]any{"restaurant_id": id, "accepting_orders": accepting})
}

func (a *app) portalAvailability(w http.ResponseWriter, r *http.Request) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	var req struct {
		AcceptingOrders bool `json:"accepting_orders"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		fail(w, 400, "INVALID_JSON", err.Error())
		return
	}
	id := r.PathValue("restaurant_id")
	_, err := a.db.Exec(r.Context(), `
		INSERT INTO restaurant_settings(restaurant_id, accepting_orders)
		VALUES($1,$2)
		ON CONFLICT(restaurant_id) DO UPDATE
		SET accepting_orders=EXCLUDED.accepting_orders, updated_at=NOW()`, id, req.AcceptingOrders)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	respond(w, 200, map[string]any{"restaurant_id": id, "accepting_orders": req.AcceptingOrders})
}

func (a *app) portalOrders(w http.ResponseWriter, r *http.Request) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	orders, err := a.listRestaurantOrders(r.Context(), r.PathValue("restaurant_id"))
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	respond(w, 200, map[string]any{"orders": orders})
}

func (a *app) listRestaurantOrders(ctx context.Context, restaurantID string) ([]restaurantOrderView, error) {
	rows, err := a.db.Query(ctx, `
		SELECT fo.order_id, fo.restaurant_id, fo.customer_id, fo.items, fo.delivery_address,
		       fo.total_amount_minor, fo.status, fo.created_at, fo.updated_at,
		       (ro.order_id IS NOT NULL) AS ticket_created
		FROM food_orders fo
		LEFT JOIN restaurant_orders ro
		  ON ro.order_id=fo.order_id AND ro.restaurant_id=fo.restaurant_id
		WHERE fo.restaurant_id=$1
		ORDER BY fo.created_at DESC
		LIMIT 100`, restaurantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := make([]restaurantOrderView, 0)
	for rows.Next() {
		var item restaurantOrderView
		var items, address []byte
		if err := rows.Scan(&item.OrderID, &item.RestaurantID, &item.CustomerID, &items,
			&address, &item.TotalAmountMinor, &item.Status, &item.CreatedAt, &item.UpdatedAt,
			&item.TicketCreated); err != nil {
			return nil, err
		}
		item.Items = json.RawMessage(items)
		item.DeliveryAddress = json.RawMessage(address)
		orders = append(orders, item)
	}
	return orders, rows.Err()
}

func (a *app) portalTransition(w http.ResponseWriter, r *http.Request, target, signal string, payload map[string]string) {
	if !a.portalAuthorized(r) {
		fail(w, 401, "UNAUTHORIZED", "portal access denied")
		return
	}
	// food_orders is the canonical order record. The Temporal workflow normally
	// creates the restaurant projection before waiting for a decision, but the
	// portal must still expose and reconcile an order if that activity is delayed.
	orderID := r.PathValue("order_id")
	restaurantID := r.PathValue("restaurant_id")
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var owner, customerID, canonicalStatus string
	var items []byte
	err = tx.QueryRow(r.Context(), `
		SELECT restaurant_id, customer_id, items, status
		FROM food_orders WHERE order_id=$1 FOR SHARE`, orderID).Scan(
		&owner, &customerID, &items, &canonicalStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "RESTAURANT_ORDER_NOT_FOUND", "order does not belong to this restaurant")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	if owner != restaurantID {
		fail(w, 404, "RESTAURANT_ORDER_NOT_FOUND", "order does not belong to this restaurant")
		return
	}

	// Rebuild the restaurant-owned projection using the canonical lifecycle
	// state so repeated portal actions remain valid even after a delayed create.
	ticketStatus := "PENDING"
	switch canonicalStatus {
	case "RESTAURANT_ACCEPTED", "DRIVER_ASSIGNED", "PREPARING":
		ticketStatus = "ACCEPTED"
	case "READY_FOR_PICKUP", "OUT_FOR_DELIVERY", "DELIVERED":
		ticketStatus = "READY"
	case "REJECTED":
		ticketStatus = "REJECTED"
	case "CANCELLED", "FAILED", "COMPENSATION_FAILED":
		ticketStatus = "CANCELLED"
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO restaurant_orders(order_id, restaurant_id, customer_id, items, status)
		VALUES($1, $2, $3, $4, $5)
		ON CONFLICT(order_id) DO NOTHING`,
		orderID, restaurantID, customerID, items, ticketStatus,
	)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	var ticketOwner string
	err = tx.QueryRow(r.Context(), `
		SELECT restaurant_id FROM restaurant_orders WHERE order_id=$1 FOR UPDATE`, orderID,
	).Scan(&ticketOwner)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	if ticketOwner != restaurantID {
		fail(w, 404, "RESTAURANT_ORDER_NOT_FOUND", "order does not belong to this restaurant")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}

	r.Header.Set("X-Service-Token", a.token)
	a.transition(w, r, target, signal, payload)
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
	id := r.PathValue("restaurant_id")
	last, err := a.latestRestaurantUpdate(r.Context(), id)
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
			updated, queryErr := a.latestRestaurantUpdate(r.Context(), id)
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

func (a *app) latestRestaurantUpdate(ctx context.Context, restaurantID string) (time.Time, error) {
	var updated time.Time
	err := a.db.QueryRow(ctx, `
		SELECT GREATEST(
			COALESCE((SELECT MAX(updated_at) FROM food_orders WHERE restaurant_id=$1), 'epoch'::timestamptz),
			COALESCE((SELECT MAX(updated_at) FROM restaurant_orders WHERE restaurant_id=$1), 'epoch'::timestamptz),
			COALESCE((SELECT updated_at FROM restaurant_settings WHERE restaurant_id=$1), 'epoch'::timestamptz)
		)`, restaurantID).Scan(&updated)
	return updated, err
}

func writePortalEvent(w http.ResponseWriter, name string, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
}
