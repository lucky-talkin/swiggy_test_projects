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

func (a *app) orderEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "STREAM_UNAVAILABLE", "streaming is not supported")
		return
	}
	orderID := r.PathValue("id")
	payload, updated, err := a.orderSnapshot(r.Context(), orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "ORDER_NOT_FOUND", "order does not exist")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	writeOrderEvent(w, payload)
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
			next, nextUpdated, queryErr := a.orderSnapshot(r.Context(), orderID)
			if errors.Is(queryErr, pgx.ErrNoRows) {
				return
			}
			if queryErr != nil {
				fmt.Fprintf(w, "event: stream-error\ndata: %q\n\n", queryErr.Error())
				flusher.Flush()
				continue
			}
			if nextUpdated.After(updated) {
				updated = nextUpdated
				writeOrderEvent(w, next)
				flusher.Flush()
			}
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func (a *app) orderSnapshot(ctx context.Context, orderID string) ([]byte, time.Time, error) {
	var id, customerID, restaurantID, currency, status, reason string
	var items, address []byte
	var total int64
	var created, updated time.Time
	err := a.db.QueryRow(ctx, `
		SELECT order_id, customer_id, restaurant_id, items, delivery_address,
		       total_amount_minor, currency, status, failure_reason, created_at, updated_at
		FROM food_orders WHERE order_id=$1`, orderID).Scan(
		&id, &customerID, &restaurantID, &items, &address, &total, &currency, &status, &reason, &created, &updated,
	)
	if err != nil {
		return nil, time.Time{}, err
	}
	payload, err := json.Marshal(map[string]any{
		"order_id": id, "customer_id": customerID, "restaurant_id": restaurantID,
		"items": json.RawMessage(items), "delivery_address": json.RawMessage(address),
		"total_amount_minor": total, "currency": currency, "status": status,
		"failure_reason": reason, "created_at": created, "updated_at": updated,
	})
	return payload, updated, err
}

func writeOrderEvent(w http.ResponseWriter, payload []byte) {
	fmt.Fprintf(w, "event: order\ndata: %s\n\n", payload)
}
