package main

import (
	"log"
	"net/http"
	"time"

	"github.com/lucasdonizete/clima-cep-otel/internal/app"
	"github.com/lucasdonizete/clima-cep-otel/internal/weathersvc"
)

func main() {
	apiKey := app.Getenv("WEATHER_API_KEY", "")
	if apiKey == "" {
		log.Fatal("WEATHER_API_KEY is required (see .env.example)")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	h := &weathersvc.Handler{
		Locations: &weathersvc.ViaCEP{
			BaseURL: app.Getenv("VIACEP_URL", "https://viacep.com.br"),
			Client:  client,
		},
		Temperatures: &weathersvc.WeatherAPI{
			BaseURL: app.Getenv("WEATHERAPI_URL", "https://api.weatherapi.com"),
			APIKey:  apiKey,
			Client:  client,
		},
	}

	if err := app.Run("service-b", ":"+app.Getenv("PORT", "8081"), h.Routes()); err != nil {
		log.Fatal(err)
	}
}
