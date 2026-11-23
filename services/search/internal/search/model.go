package search

import "context"

type SearchRequest struct {
	Query string
	Type string
	City string
	Cuisine string
	MinRating float64
	Latitude *float64
	Longitude *float64
	RadiusKM float64
	Page int
	PageSize int
}

type Document struct {
	ID string `json:"id"`
	EntityType string `json:"entity_type"`
	RestaurantID string `json:"restaurant_id,omitempty"`
	Name string `json:"name"`
	Description string `json:"description,omitempty"`
	Cuisines []string `json:"cuisines,omitempty"`
	Category string `json:"category,omitempty"`
	PriceForTwo int `json:"price_for_two,omitempty"`
	Rating float64 `json:"rating,omitempty"`
	DeliveryTimeMinutes int `json:"delivery_time_minutes,omitempty"`
	IsOpen bool `json:"is_open"`
	City string `json:"city,omitempty"`
	Area string `json:"area,omitempty"`
	Location *GeoPoint `json:"location,omitempty"`
	PopularityScore float64 `json:"popularity_score,omitempty"`
}

type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type SearchResult struct {
	Total int64 `json:"total"`
	Page int `json:"page"`
	PageSize int `json:"page_size"`
	Items []Document `json:"items"`
	TookMS int64 `json:"took_ms"`
}

type esSearchResponse struct {
	Took int64 `json:"took"`
	Hits struct {
		Total struct { Value int64 `json:"value"` } `json:"total"`
		Hits []struct { Source Document `json:"_source"` } `json:"hits"`
	} `json:"hits"`
}

type Searcher interface {
	Search(ctx context.Context, request SearchRequest) (SearchResult, error)
	Ping(ctx context.Context) error
}
