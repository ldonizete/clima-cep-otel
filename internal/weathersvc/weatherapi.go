package weathersvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// WeatherAPI fetches the current temperature using weatherapi.com.
//
// The API key travels in the query string, so this client deliberately uses
// a plain http.Client (no otelhttp transport) and strips the URL from
// transport errors: neither spans nor logs may carry the key.
type WeatherAPI struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

type weatherAPIResponse struct {
	Current struct {
		TempC *float64 `json:"temp_c"`
	} `json:"current"`
}

// Celsius returns the current temperature of city inside a "weatherapi.lookup" span.
func (w *WeatherAPI) Celsius(ctx context.Context, city string) (float64, error) {
	ctx, span := otel.Tracer("service-b").Start(ctx, "weatherapi.lookup",
		trace.WithAttributes(attribute.String("city", city)))
	defer span.End()

	temp, err := w.fetch(ctx, city)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	span.SetAttributes(attribute.Float64("temp_c", temp))
	return temp, nil
}

func (w *WeatherAPI) fetch(ctx context.Context, city string) (float64, error) {
	q := url.Values{}
	q.Set("key", w.APIKey)
	q.Set("q", city)
	q.Set("aqi", "no")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.BaseURL+"/v1/current.json?"+q.Encode(), nil)
	if err != nil {
		return 0, errors.New("build weatherapi request")
	}

	resp, err := w.Client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, fmt.Errorf("weatherapi request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("weatherapi returned status %d", resp.StatusCode)
	}

	var body weatherAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode weatherapi response: %w", err)
	}
	if body.Current.TempC == nil {
		return 0, errors.New("weatherapi response has no temp_c")
	}
	return *body.Current.TempC, nil
}
