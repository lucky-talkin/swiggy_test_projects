package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucky-talkin/swiggy_test_projects/services/search/internal/search"
)

type fakeBackend struct{}
func (fakeBackend) Ping(context.Context) error { return nil }
func (fakeBackend) Search(context.Context, search.SearchRequest) (search.SearchResult, error) {
	return search.SearchResult{Total: 0, Page: 1, PageSize: 20, Items: []search.Document{}}, nil
}

func TestSearchEndpoint(t *testing.T) {
	backend := &fakeBackend{}
	handler := NewHandler(search.NewService(backend), backend)
	request := httptest.NewRequest(http.MethodGet, "/v1/search?q=pizza&city=Bengaluru&type=menu_item&restaurant_id=restaurant-1", nil)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK { t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String()) }
	if response.Header().Get("X-Request-ID") == "" { t.Fatal("expected request ID response header") }
	if backend.request.RestaurantID != "restaurant-1" || backend.request.Type != "menu_item" { t.Fatalf("restaurant filter not forwarded: %+v", backend.request) }
}

func TestReadyEndpoint(t *testing.T) {
	backend := &fakeBackend{}
	handler := NewHandler(search.NewService(backend), backend)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK { t.Fatalf("expected 200, got %d", response.Code) }
}
