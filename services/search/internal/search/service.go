package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid search request")

type Service struct {
	searcher Searcher
}

func NewService(searcher Searcher) *Service {
	return &Service{searcher: searcher}
}

func (s *Service) Search(ctx context.Context, request SearchRequest) (SearchResult, error) {
	request.Query = strings.TrimSpace(request.Query)
	request.City = strings.ToLower(strings.TrimSpace(request.City))
	request.Cuisine = strings.ToLower(strings.TrimSpace(request.Cuisine))
	request.Type = strings.ToLower(strings.TrimSpace(request.Type))

	if len(request.Query) > 120 {
		return SearchResult{}, fmt.Errorf("%w: q must be at most 120 characters", ErrInvalidRequest)
	}
	if request.Type != "" && request.Type != "restaurant" && request.Type != "menu_item" {
		return SearchResult{}, fmt.Errorf("%w: type must be restaurant or menu_item", ErrInvalidRequest)
	}
	if request.MinRating < 0 || request.MinRating > 5 {
		return SearchResult{}, fmt.Errorf("%w: min_rating must be between 0 and 5", ErrInvalidRequest)
	}
	if request.Page == 0 {
		request.Page = 1
	}
	if request.Page < 1 || request.Page > 10000 {
		return SearchResult{}, fmt.Errorf("%w: page must be between 1 and 10000", ErrInvalidRequest)
	}
	if request.PageSize == 0 {
		request.PageSize = 20
	}
	if request.PageSize < 1 || request.PageSize > 50 {
		return SearchResult{}, fmt.Errorf("%w: page_size must be between 1 and 50", ErrInvalidRequest)
	}
	if (request.Latitude == nil) != (request.Longitude == nil) {
		return SearchResult{}, fmt.Errorf("%w: lat and lon must be provided together", ErrInvalidRequest)
	}
	if request.Latitude != nil {
		if *request.Latitude < -90 || *request.Latitude > 90 || *request.Longitude < -180 || *request.Longitude > 180 {
			return SearchResult{}, fmt.Errorf("%w: invalid coordinates", ErrInvalidRequest)
		}
		if request.RadiusKM == 0 {
			request.RadiusKM = 5
		}
		if request.RadiusKM < 0.1 || request.RadiusKM > 50 {
			return SearchResult{}, fmt.Errorf("%w: radius_km must be between 0.1 and 50", ErrInvalidRequest)
		}
	}
	return s.searcher.Search(ctx, request)
}
