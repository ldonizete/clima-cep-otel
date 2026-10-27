// Package app runs an HTTP service with tracing and graceful shutdown.
package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/ldonizete/clima-cep-otel/internal/telemetry"
)

// Run starts handler on addr, instrumented as serviceName, and blocks until
// SIGINT/SIGTERM. Spans are flushed before it returns.
func Run(serviceName, addr string, handler http.Handler) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := telemetry.Init(ctx, serviceName)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           otelhttp.NewHandler(handler, serviceName),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("%s listening on %s", serviceName, addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Printf("%s shutting down", serviceName)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	return shutdownTracing(shutdownCtx)
}

// Getenv returns the environment variable key, or fallback when it is empty.
func Getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
