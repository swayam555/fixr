package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"fixr.in/svc/internal/geo"
	"fixr.in/svc/internal/match"
)

type EtaPair struct {
	ProLat, ProLng, OrderLat, OrderLng float64
	DistanceM                          float64
	HourOfDay                          int
	Skill                              string
	ProJobsToday                       int
}

// EtaModel predicts travel time; ai-service serves the LightGBM model.
type EtaModel interface {
	Predict(ctx context.Context, pairs []EtaPair) ([]float64, string, error)
}

type HTTPEta struct {
	URL    string
	Client *http.Client
}

type etaRequest struct {
	Pairs []map[string]any `json:"pairs"`
}

type etaResponse struct {
	EtaSec []float64 `json:"etaSecP50"`
	Model  string    `json:"model"`
}

func (h *HTTPEta) Predict(ctx context.Context, pairs []EtaPair) ([]float64, string, error) {
	req := etaRequest{Pairs: make([]map[string]any, len(pairs))}
	for i, p := range pairs {
		req.Pairs[i] = map[string]any{
			"proLat": p.ProLat, "proLng": p.ProLng, "orderLat": p.OrderLat, "orderLng": p.OrderLng,
			"distanceM": p.DistanceM, "hourOfDay": p.HourOfDay, "skill": p.Skill, "proJobsToday": p.ProJobsToday,
		}
	}
	body, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(r)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("eta: status %d", resp.StatusCode)
	}
	var out etaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.EtaSec) != len(pairs) {
		return nil, "", fmt.Errorf("eta: bad response")
	}
	return out.EtaSec, out.Model, nil
}

// WithFallback never fails a round: if the model is down or slow, dispatch uses the heuristic.
func WithFallback(ctx context.Context, m EtaModel, pairs []EtaPair) ([]float64, string) {
	if m != nil && len(pairs) > 0 {
		if eta, name, err := m.Predict(ctx, pairs); err == nil {
			return eta, name
		}
	}
	out := make([]float64, len(pairs))
	for i, p := range pairs {
		out[i] = match.HeuristicEtaSec(geo.Point{Lat: p.ProLat, Lng: p.ProLng}, geo.Point{Lat: p.OrderLat, Lng: p.OrderLng})
	}
	return out, "heuristic"
}
