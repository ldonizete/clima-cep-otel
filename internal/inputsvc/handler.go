// Package inputsvc is the entry service (Service A): validates the CEP and forwards it to Service B.
package inputsvc

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/ldonizete/clima-cep-otel/internal/cep"
)

const maxBodyBytes = 1 << 20

// Handler serves POST / with {"cep": "<8 digits>"}.
type Handler struct {
	// WeatherURL is the base URL of Service B, e.g. http://service-b:8081.
	WeatherURL string
	// Client must carry the otelhttp transport so the trace context propagates.
	Client *http.Client
}

// Routes returns the service router.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /{$}", h.lookup)
	return mux
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	zip, ok := decodeCEP(w, r)
	if !ok || !cep.Valid(zip) {
		writeText(w, http.StatusUnprocessableEntity, "invalid zipcode")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		h.WeatherURL+"/weather/"+url.PathEscape(zip), nil)
	if err != nil {
		writeText(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		log.Printf("service b unreachable: %v", err)
		writeText(w, http.StatusBadGateway, "weather service unavailable")
		return
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// decodeCEP extracts the "cep" field and reports false unless it is a JSON string.
func decodeCEP(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		CEP json.RawMessage `json:"cep"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil {
		return "", false
	}
	var zip string
	if err := json.Unmarshal(body.CEP, &zip); err != nil {
		return "", false
	}
	return zip, true
}

func writeText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}
