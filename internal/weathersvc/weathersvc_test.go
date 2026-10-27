package weathersvc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type fakeLocations struct {
	city string
	err  error
}

func (f fakeLocations) City(context.Context, string) (string, error) { return f.city, f.err }

type fakeTemps struct {
	c   float64
	err error
}

func (f fakeTemps) Celsius(context.Context, string) (float64, error) { return f.c, f.err }

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		locs       fakeLocations
		temps      fakeTemps
		wantStatus int
		wantBody   string
	}{
		{
			name:       "success",
			path:       "/weather/29902555",
			locs:       fakeLocations{city: "São Paulo"},
			temps:      fakeTemps{c: 28.5},
			wantStatus: http.StatusOK,
			wantBody:   `{"city":"São Paulo","temp_C":28.5,"temp_F":83.3,"temp_K":301.5}`,
		},
		{
			name:       "malformed cep",
			path:       "/weather/123",
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "invalid zipcode",
		},
		{
			name:       "cep not found",
			path:       "/weather/00000000",
			locs:       fakeLocations{err: ErrZipcodeNotFound},
			wantStatus: http.StatusNotFound,
			wantBody:   "can not find zipcode",
		},
		{
			name:       "location says invalid",
			path:       "/weather/12345678",
			locs:       fakeLocations{err: ErrInvalidZipcode},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "invalid zipcode",
		},
		{
			name:       "weather upstream failure",
			path:       "/weather/29902555",
			locs:       fakeLocations{city: "Linhares"},
			temps:      fakeTemps{err: errors.New("boom")},
			wantStatus: http.StatusBadGateway,
			wantBody:   "upstream service unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := (&Handler{Locations: tt.locs, Temperatures: tt.temps}).Routes()
			rec := get(t, h, tt.path)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func TestViaCEP(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantCity string
		wantErr  error
	}{
		{"found", 200, `{"localidade":"Linhares"}`, "Linhares", nil},
		{"erro bool", 200, `{"erro":true}`, "", ErrZipcodeNotFound},
		{"erro string", 200, `{"erro":"true"}`, "", ErrZipcodeNotFound},
		{"empty city", 200, `{}`, "", ErrZipcodeNotFound},
		{"bad request", 400, ``, "", ErrInvalidZipcode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			v := &ViaCEP{BaseURL: srv.URL, Client: srv.Client()}
			city, err := v.City(context.Background(), "29902555")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if city != tt.wantCity {
				t.Errorf("city = %q, want %q", city, tt.wantCity)
			}
		})
	}
}

func TestWeatherAPI(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		if r.URL.Query().Get("key") != "secret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"current":{"temp_c":28.5}}`))
	}))
	defer srv.Close()

	w := &WeatherAPI{BaseURL: srv.URL, APIKey: "secret", Client: srv.Client()}

	temp, err := w.Celsius(context.Background(), "São Paulo")
	if err != nil {
		t.Fatal(err)
	}
	if temp != 28.5 {
		t.Errorf("temp = %v, want 28.5", temp)
	}
	if gotQuery != "São Paulo" {
		t.Errorf("city reached upstream as %q, want %q", gotQuery, "São Paulo")
	}

	bad := &WeatherAPI{BaseURL: srv.URL, APIKey: "wrong", Client: srv.Client()}
	if _, err := bad.Celsius(context.Background(), "x"); err == nil {
		t.Error("expected error for rejected key")
	}
}

func TestWeatherAPIErrorDoesNotLeakKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // connection will be refused

	w := &WeatherAPI{BaseURL: url, APIKey: "topsecret", Client: &http.Client{Timeout: time.Second}}
	_, err := w.Celsius(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "topsecret") {
		t.Errorf("error leaks api key: %v", err)
	}
}

func TestManualSpans(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))

	viacep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"localidade":"Linhares"}`))
	}))
	defer viacep.Close()
	weather := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"current":{"temp_c":30}}`))
	}))
	defer weather.Close()

	h := (&Handler{
		Locations:    &ViaCEP{BaseURL: viacep.URL, Client: viacep.Client()},
		Temperatures: &WeatherAPI{BaseURL: weather.URL, APIKey: "k", Client: weather.Client()},
	}).Routes()

	if rr := get(t, h, "/weather/29902555"); rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}

	names := map[string]bool{}
	for _, s := range rec.Ended() {
		names[s.Name()] = true
	}
	for _, want := range []string{"viacep.lookup", "weatherapi.lookup"} {
		if !names[want] {
			t.Errorf("span %q not recorded; got %v", want, names)
		}
	}
}
