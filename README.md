# Clima por CEP com Observabilidade

Sistema distribuído em Go com dois microsserviços que consultam a temperatura de uma cidade a partir do CEP, com rastreamento distribuído via **OpenTelemetry**, **OTEL Collector** e **Zipkin**.

```
cliente ──POST /──▶ Serviço A (valida CEP) ──GET /weather/{cep}──▶ Serviço B ──▶ ViaCEP
                          │                                           │    └──▶ WeatherAPI
                          └────────────── OTLP/gRPC ──▶ OTEL Collector ◀──────┘
                                                              │
                                                              ▼
                                                           Zipkin
```

| Serviço | Porta | Papel |
|---|---|---|
| `service-a` | 8080 | Recebe `POST` com o CEP, valida o formato e encaminha ao B |
| `service-b` | 8081 | Consulta a cidade (ViaCEP) e a temperatura (WeatherAPI) e converte |
| `otel-collector` | 4317 / 4318 | Recebe os traces (OTLP) e exporta para o Zipkin |
| `zipkin` | 9411 | Visualização dos traces |

## Pré-requisitos

- Docker e Docker Compose
- Uma chave gratuita da [WeatherAPI](https://www.weatherapi.com/)

## Como executar

```bash
cp .env.example .env
# edite o .env e preencha WEATHER_API_KEY
docker compose up --build
```

## Como fazer a requisição no Serviço A

```bash
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"cep": "29902555"}'
```

Resposta de sucesso (`200`):

```json
{ "city": "Linhares", "temp_C": 28.5, "temp_F": 83.3, "temp_K": 301.5 }
```

Conversões: `F = C × 1.8 + 32` e `K = C + 273`.

| Situação | Status | Corpo |
|---|---|---|
| CEP com formato inválido (não é string, ou não tem 8 dígitos) | `422` | `invalid zipcode` |
| CEP válido, mas inexistente | `404` | `can not find zipcode` |
| Falha em ViaCEP, WeatherAPI ou no Serviço B | `502` | mensagem genérica (fora da especificação) |

Exemplos de erro:

```bash
curl -i -X POST http://localhost:8080 -d '{"cep": "123"}'        # 422
curl -i -X POST http://localhost:8080 -d '{"cep": 29902555}'     # 422 (número, não string)
curl -i -X POST http://localhost:8080 -d '{"cep": "00000000"}'   # 404
```

O Serviço B também pode ser chamado direto: `curl http://localhost:8081/weather/29902555`.

## Como ver os traces no Zipkin

1. Faça uma ou mais requisições como acima.
2. Abra <http://localhost:9411> e clique em **Run Query** (ou filtre por `serviceName` = `service-a`).
3. Abra um trace. O fluxo completo aparece em um único trace:

   | Span | Serviço | Origem |
   |---|---|---|
   | `post /{$}` | service-a | automático (`otelhttp`, requisição recebida) |
   | `http get` | service-a | automático (`otelhttp`, chamada ao B) |
   | `get /weather/{cep}` | service-b | automático (`otelhttp`, requisição recebida) |
   | `viacep.lookup` | service-b | **manual**: busca de CEP |
   | `weatherapi.lookup` | service-b | **manual**: busca de temperatura |

Requisições rejeitadas com `422` no Serviço A geram apenas os spans do A, pois não chegam ao B.

## Testes

```bash
go test ./...
```

Cobrem validação do CEP, conversões, mapeamento de erros, os clientes das APIs externas (com servidores falsos), a criação dos spans manuais e a propagação do trace entre A e B.

## Estrutura

```
cmd/service-a, cmd/service-b   pontos de entrada
internal/inputsvc              Serviço A: validação e encaminhamento
internal/weathersvc            Serviço B: ViaCEP, WeatherAPI, conversões
internal/telemetry             SDK OpenTelemetry (OTLP gRPC + W3C trace context)
internal/app                   servidor HTTP com shutdown gracioso
otel-collector-config.yaml     receiver OTLP → exporter Zipkin
```

A chave da WeatherAPI é enviada na query string da API deles; por isso o cliente dessa chamada não usa o transporte `otelhttp`, e os erros de rede são sanitizados, para que a chave nunca apareça nos spans ou logs.
