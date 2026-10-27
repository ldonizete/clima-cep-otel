// Package weathersvc is the orchestration service (Service B): CEP -> city -> temperature.
package weathersvc

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/lucasdonizete/clima-cep-otel/internal/cep"
)

// LocationFinder resolves a CEP to a city name.
type LocationFinder interface {
	City(ctx context.Context, cep string) (string, error)
}

// TemperatureFinder returns the current temperature of a city in Celsius.
type TemperatureFinder interface {
	Celsius(ctx context.Context, city string) (float64, error)
}

// Response is the successful payload.
type Response struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

// Handler serves GET /weather/{cep}.
type Handler struct {
	Locations    LocationFinder
	Temperatures TemperatureFinder
}

// Routes returns the service router.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /weather/{cep}", h.weather)
	return mux
}

func (h *Handler) weather(w http.ResponseWriter, r *http.Request) {
	zip := r.PathValue("cep")
	if !cep.Valid(zip) {
		writeText(w, http.StatusUnprocessableEntity, ErrInvalidZipcode.Error())
		return
	}

	city, err := h.Locations.City(r.Context(), zip)
	if err != nil {
		h.fail(w, err)
		return
	}

	celsius, err := h.Temperatures.Celsius(r.Context(), city)
	if err != nil {
		h.fail(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{
		City:  city,
		TempC: celsius,
		TempF: ToFahrenheit(celsius),
		TempK: ToKelvin(celsius),
	})
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidZipcode):
		writeText(w, http.StatusUnprocessableEntity, ErrInvalidZipcode.Error())
	case errors.Is(err, ErrZipcodeNotFound):
		writeText(w, http.StatusNotFound, ErrZipcodeNotFound.Error())
	default:
		log.Printf("upstream failure: %v", err)
		writeText(w, http.StatusBadGateway, "upstream service unavailable")
	}
}

func writeText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}
