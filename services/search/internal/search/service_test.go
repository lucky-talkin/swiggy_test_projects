package search

import (
	"context"
	"errors"
	"testing"
)

type fakeSearcher struct {
	request SearchRequest
	err error
}

func (f *fakeSearcher) Search(_ context.Context, request SearchRequest) (SearchResult, error) {
	f.request = request
	return SearchResult{Page: request.Page, PageSize: request.PageSize, Items: []Document{}}, f.err
}
func (f *fakeSearcher) Ping(context.Context) error { return nil }

func TestServiceAppliesDefaultsAndNormalizesFilters(t *testing.T) {
	fake := &fakeSearcher{}
	service := NewService(fake)
	_, err := service.Search(context.Background(), SearchRequest{Query: "  pizza ", City: " Bengaluru ", Cuisine: " South Indian "})
	if err != nil { t.Fatal(err) }
	if fake.request.Query != "pizza" || fake.request.City != "bengaluru" || fake.request.Cuisine != "south indian" {
		t.Fatalf("unexpected normalized request: %+v", fake.request)
	}
	if fake.request.Page != 1 || fake.request.PageSize != 20 {
		t.Fatalf("unexpected pagination defaults: %+v", fake.request)
	}
}

func TestServiceRejectsInvalidType(t *testing.T) {
	service := NewService(&fakeSearcher{})
	_, err := service.Search(context.Background(), SearchRequest{Type: "driver"})
	if !errors.Is(err, ErrInvalidRequest) { t.Fatalf("expected ErrInvalidRequest, got %v", err) }
}

func TestServiceRequiresCoordinatePair(t *testing.T) {
	lat := 12.9
	service := NewService(&fakeSearcher{})
	_, err := service.Search(context.Background(), SearchRequest{Latitude: &lat})
	if !errors.Is(err, ErrInvalidRequest) { t.Fatalf("expected ErrInvalidRequest, got %v", err) }
}

func TestServiceRejectsPageBeyondElasticsearchWindow(t *testing.T) {
	service := NewService(&fakeSearcher{})
	_, err := service.Search(context.Background(), SearchRequest{Page: 201, PageSize: 50})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}
