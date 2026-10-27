package inputsvc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func fakeServiceB(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		switch r.URL.Path {
		case "/weather/29902555":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"city":"Linhares","temp_C":30,"temp_F":86,"temp_K":303}`))
		default:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("can not find zipcode"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInvalidInputNeverReachesServiceB(t *testing.T) {
	calls := 0
	b := fakeServiceB(t, &calls)
	h := (&Handler{WeatherURL: b.URL, Client: b.Client()}).Routes()

	invalid := map[string]string{
		"too short":   `{"cep":"2990255"}`,
		"too long":    `{"cep":"299025555"}`,
		"hyphen":      `{"cep":"29902-555"}`,
		"letters":     `{"cep":"abcdefgh"}`,
		"number":      `{"cep":29902555}`,
		"null":        `{"cep":null}`,
		"missing":     `{}`,
		"array":       `{"cep":["29902555"]}`,
		"not json":    `29902555`,
		"empty body":  ``,
		"wrong field": `{"zipcode":"29902555"}`,
	}
	for name, body := range invalid {
		t.Run(name, func(t *testing.T) {
			rec := post(h, body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422", rec.Code)
			}
			if rec.Body.String() != "invalid zipcode" {
				t.Errorf("body = %q, want %q", rec.Body.String(), "invalid zipcode")
			}
		})
	}
	if calls != 0 {
		t.Errorf("service B was called %d times for invalid input", calls)
	}
}

func TestForwardsServiceBResponse(t *testing.T) {
	calls := 0
	b := fakeServiceB(t, &calls)
	h := (&Handler{WeatherURL: b.URL, Client: b.Client()}).Routes()

	rec := post(h, `{"cep":"29902555"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if want := `{"city":"Linhares","temp_C":30,"temp_F":86,"temp_K":303}`; rec.Body.String() != want {
		t.Errorf("body = %q, want %q", rec.Body.String(), want)
	}

	rec = post(h, `{"cep":"00000000"}`)
	if rec.Code != http.StatusNotFound || rec.Body.String() != "can not find zipcode" {
		t.Errorf("got %d %q, want 404 %q", rec.Code, rec.Body.String(), "can not find zipcode")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := (&Handler{WeatherURL: "http://unused", Client: http.DefaultClient}).Routes()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestServiceBDown(t *testing.T) {
	b := httptest.NewServer(http.NotFoundHandler())
	url := b.URL
	b.Close()

	h := (&Handler{WeatherURL: url, Client: http.DefaultClient}).Routes()
	if rec := post(h, `{"cep":"29902555"}`); rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

// The trace started at A must continue inside B under the same trace ID,
// with B's server span parented on A's client span.
func TestTracePropagatesFromAToB(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	otel.SetTextMapPropagator(propagation.TraceContext{})

	b := httptest.NewServer(otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}), "service-b"))
	defer b.Close()

	a := otelhttp.NewHandler((&Handler{
		WeatherURL: b.URL,
		Client:     &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}).Routes(), "service-a")

	if got := post(a, `{"cep":"29902555"}`); got.Code != http.StatusOK {
		t.Fatalf("status = %d", got.Code)
	}

	var serverA, serverB, clientA sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		switch {
		case s.SpanKind() == trace.SpanKindClient:
			clientA = s
		case s.SpanKind() == trace.SpanKindServer && !s.Parent().IsValid():
			serverA = s
		case s.SpanKind() == trace.SpanKindServer:
			serverB = s
		}
	}
	if serverA == nil || serverB == nil || clientA == nil {
		t.Fatalf("want server A, client A and server B spans; got A=%v clientA=%v B=%v",
			serverA != nil, clientA != nil, serverB != nil)
	}

	traceID := serverA.SpanContext().TraceID()
	if clientA.SpanContext().TraceID() != traceID || serverB.SpanContext().TraceID() != traceID {
		t.Error("spans of A and B are in different traces")
	}
	if clientA.Parent().SpanID() != serverA.SpanContext().SpanID() {
		t.Error("A's client span is not a child of A's server span")
	}
	if serverB.Parent().SpanID() != clientA.SpanContext().SpanID() {
		t.Error("B's server span is not a child of A's client span")
	}
}
