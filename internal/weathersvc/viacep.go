package weathersvc

import (
	"bytes"
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

var (
	// ErrInvalidZipcode means the CEP has a wrong format.
	ErrInvalidZipcode = errors.New("invalid zipcode")
	// ErrZipcodeNotFound means the CEP is well formed but does not exist.
	ErrZipcodeNotFound = errors.New("can not find zipcode")
)

// ViaCEP resolves a CEP to a city name using the ViaCEP API.
type ViaCEP struct {
	BaseURL string
	Client  *http.Client
}

// flexBool decodes ViaCEP's "erro" field, which arrives as true or "true".
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch string(bytes.Trim(data, `"`)) {
	case "true":
		*b = true
	default:
		*b = false
	}
	return nil
}

type viaCEPResponse struct {
	Localidade string   `json:"localidade"`
	Erro       flexBool `json:"erro"`
}

// City returns the city for cep inside a "viacep.lookup" span.
func (v *ViaCEP) City(ctx context.Context, cep string) (string, error) {
	ctx, span := otel.Tracer("service-b").Start(ctx, "viacep.lookup",
		trace.WithAttributes(attribute.String("cep", cep)))
	defer span.End()

	city, err := v.lookup(ctx, cep)
	switch {
	case errors.Is(err, ErrZipcodeNotFound), errors.Is(err, ErrInvalidZipcode):
		span.SetAttributes(attribute.Bool("cep.found", false))
	case err != nil:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	default:
		span.SetAttributes(attribute.Bool("cep.found", true), attribute.String("city", city))
	}
	return city, err
}

func (v *ViaCEP) lookup(ctx context.Context, cep string) (string, error) {
	endpoint := fmt.Sprintf("%s/ws/%s/json/", v.BaseURL, url.PathEscape(cep))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := v.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("viacep request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest:
		return "", ErrInvalidZipcode
	case http.StatusNotFound:
		return "", ErrZipcodeNotFound
	default:
		return "", fmt.Errorf("viacep returned status %d", resp.StatusCode)
	}

	var body viaCEPResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode viacep response: %w", err)
	}
	if bool(body.Erro) || body.Localidade == "" {
		return "", ErrZipcodeNotFound
	}
	return body.Localidade, nil
}
