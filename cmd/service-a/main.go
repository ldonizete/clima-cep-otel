package main

import (
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/lucasdonizete/clima-cep-otel/internal/app"
	"github.com/lucasdonizete/clima-cep-otel/internal/inputsvc"
)

func main() {
	h := &inputsvc.Handler{
		WeatherURL: app.Getenv("SERVICE_B_URL", "http://localhost:8081"),
		Client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}

	if err := app.Run("service-a", ":"+app.Getenv("PORT", "8080"), h.Routes()); err != nil {
		log.Fatal(err)
	}
}
