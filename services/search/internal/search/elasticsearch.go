package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ElasticsearchClient struct {
	baseURL string
	index   string
	client  *http.Client
}

func NewElasticsearchClient(baseURL, index string, timeout time.Duration) *ElasticsearchClient {
	return &ElasticsearchClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		index:   index,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *ElasticsearchClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to Elasticsearch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Elasticsearch returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ElasticsearchClient) Search(ctx context.Context, request SearchRequest) (SearchResult, error) {
	from := (request.Page - 1) * request.PageSize
	boolQuery := map[string]any{"filter": []any{}}
	filters := boolQuery["filter"].([]any)

	if request.Type != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"entity_type": request.Type}})
	}
	if request.City != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"city.keyword": strings.ToLower(request.City)}})
	}
	if request.RestaurantID != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"restaurant_id": request.RestaurantID}})
	}
	if request.Cuisine != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"cuisines.keyword": strings.ToLower(request.Cuisine)}})
	}
	if request.MinRating > 0 {
		filters = append(filters, map[string]any{"range": map[string]any{"rating": map[string]any{"gte": request.MinRating}}})
	}
	filters = append(filters, map[string]any{"term": map[string]any{"is_open": true}})
	boolQuery["filter"] = filters

	if request.Query != "" {
		boolQuery["must"] = []any{map[string]any{
			"multi_match": map[string]any{
				"query": request.Query,
				"fields": []string{"name^4", "cuisines^3", "category^2", "description", "area"},
				"type": "best_fields",
				"fuzziness": "AUTO",
			},
		}}
	} else {
		boolQuery["must"] = []any{map[string]any{"match_all": map[string]any{}}}
	}

	if request.Latitude != nil && request.Longitude != nil {
		filters = append(filters, map[string]any{
			"geo_distance": map[string]any{
				"distance": fmt.Sprintf("%gkm", request.RadiusKM),
				"location": map[string]float64{"lat": *request.Latitude, "lon": *request.Longitude},
			},
		})
		boolQuery["filter"] = filters
	}

	payload := map[string]any{
		"from": from,
		"size": request.PageSize,
		"track_total_hits": true,
		"query": map[string]any{"bool": boolQuery},
		"sort": []any{
			map[string]any{"_score": map[string]any{"order": "desc"}},
			map[string]any{"popularity_score": map[string]any{"order": "desc", "missing": "_last"}},
			map[string]any{"id": map[string]any{"order": "asc"}},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return SearchResult{}, fmt.Errorf("encode Elasticsearch query: %w", err)
	}
	endpoint := c.baseURL + "/" + url.PathEscape(c.index) + "/_search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return SearchResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return SearchResult{}, fmt.Errorf("execute Elasticsearch search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SearchResult{}, fmt.Errorf("Elasticsearch search returned status %d", resp.StatusCode)
	}
	var decoded esSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return SearchResult{}, fmt.Errorf("decode Elasticsearch response: %w", err)
	}
	items := make([]Document, 0, len(decoded.Hits.Hits))
	for _, hit := range decoded.Hits.Hits {
		items = append(items, hit.Source)
	}
	return SearchResult{Total: decoded.Hits.Total.Value, Page: request.Page, PageSize: request.PageSize, Items: items, TookMS: decoded.Took}, nil
}
