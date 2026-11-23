package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestElasticsearchSearchBuildsQueryAndDecodesHits(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/food-search-v1/_search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content type: %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode query body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"took": 7,
			"hits": {
				"total": {"value": 1, "relation": "eq"},
				"hits": [{
					"_index": "food-search-v1",
					"_id": "restaurant-123",
					"_score": 4.2,
					"_source": {
						"id": "restaurant-123",
						"entity_type": "restaurant",
						"name": "Dosa House",
						"cuisines": ["south indian"],
						"rating": 4.5,
						"is_open": true,
						"city": "bengaluru",
						"location": {"lat": 12.9784, "lon": 77.6408}
					}
				}]
			}
		}`)
	}))
	defer server.Close()

	client := NewElasticsearchClient(server.URL, "food-search-v1", time.Second)
	lat, lon := 12.9716, 77.5946
	result, err := client.Search(context.Background(), SearchRequest{
		Query: "dosa", Type: "restaurant", City: "bengaluru", Cuisine: "south indian",
		MinRating: 4, Latitude: &lat, Longitude: &lon, RadiusKM: 5, Page: 2, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result.Total != 1 || result.Page != 2 || result.PageSize != 10 || result.TookMS != 7 {
		t.Fatalf("unexpected result metadata: %+v", result)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "Dosa House" || result.Items[0].Rating != 4.5 {
		t.Fatalf("unexpected result items: %+v", result.Items)
	}

	if got := captured["from"]; got != float64(10) {
		t.Errorf("from = %v, want 10", got)
	}
	if got := captured["size"]; got != float64(10) {
		t.Errorf("size = %v, want 10", got)
	}
	if got := captured["track_total_hits"]; got != true {
		t.Errorf("track_total_hits = %v, want true", got)
	}

	query := captured["query"].(map[string]any)
	boolQuery := query["bool"].(map[string]any)
	must := boolQuery["must"].([]any)
	multiMatch := must[0].(map[string]any)["multi_match"].(map[string]any)
	if multiMatch["query"] != "dosa" || multiMatch["fuzziness"] != "AUTO" {
		t.Errorf("unexpected full-text query: %+v", multiMatch)
	}

	filters := boolQuery["filter"].([]any)
	filterJSON, _ := json.Marshal(filters)
	for _, expected := range []string{
		"\"entity_type\":\"restaurant\"",
		"\"city.keyword\":\"bengaluru\"",
		"\"cuisines.keyword\":\"south indian\"",
		"\"gte\":4",
		"\"is_open\":true",
		"\"distance\":\"5km\"",
		"\"lat\":12.9716",
		"\"lon\":77.5946",
	} {
		if !strings.Contains(string(filterJSON), expected) {
			t.Errorf("query filters missing %s: %s", expected, filterJSON)
		}
	}
	sortJSON, _ := json.Marshal(captured["sort"])
	if !strings.Contains(string(sortJSON), "\"_score\"") || !strings.Contains(string(sortJSON), "\"popularity_score\"") {
		t.Errorf("expected relevance and popularity sorting, got %s", sortJSON)
	}
}

func TestElasticsearchSearchBrowseUsesMatchAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode query: %v", err)
		}
		boolQuery := payload["query"].(map[string]any)["bool"].(map[string]any)
		must := boolQuery["must"].([]any)
		if _, ok := must[0].(map[string]any)["match_all"]; !ok {
			t.Errorf("expected match_all query, got %+v", must)
		}
		_, _ = fmt.Fprint(w, `{"took":1,"hits":{"total":{"value":0},"hits":[]}}`)
	}))
	defer server.Close()

	client := NewElasticsearchClient(server.URL, "food-search-v1", time.Second)
	result, err := client.Search(context.Background(), SearchRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result.Total != 0 || result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("expected an empty, non-nil result set, got %+v", result)
	}
}

func TestElasticsearchSearchReturnsBackendStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "index not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := NewElasticsearchClient(server.URL, "missing-index", time.Second)
	_, err := client.Search(context.Background(), SearchRequest{Page: 1, PageSize: 20})
	if err == nil || !strings.Contains(err.Error(), "status 404") {
		t.Fatalf("expected status 404 error, got %v", err)
	}
}

func TestElasticsearchPingChecksBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"name":"test-node","cluster_name":"test","version":{"number":"8.15.3"},"tagline":"You Know, for Search"}`)
	}))
	defer server.Close()

	client := NewElasticsearchClient(server.URL, "food-search-v1", time.Second)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}
