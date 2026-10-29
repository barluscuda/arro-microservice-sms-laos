# Arro Microservice: SMS Laos

Go SMS service using Echo, PostgreSQL raw SQL, Zap, Viper, and the Telbiz Go SDK.
The API listens on port `3285` by default. Authentication and authorization are
expected from the internal API gateway; this service does not implement them.

## Run

Required environment variables:

| Name | Purpose |
| --- | --- |
| `DATABASE_URL` | PostgreSQL connection URL |
| `TELBIZ_CLIENT_ID` | Telbiz client ID |
| `TELBIZ_CLIENT_SECRET` | Telbiz client secret |

Run the migration once, then start the service:

```sh
psql "$DATABASE_URL" -f migrations/001_create_sms_transactions.sql
go run ./cmd/sms-api
```

`PORT` defaults to `3285`. Connection pool sizes and HTTP, SDK, startup, and
shutdown timeouts have defaults in `internal/config/config.go`.

To run the container stack, provide `POSTGRES_PASSWORD`, `TELBIZ_CLIENT_ID`, and
`TELBIZ_CLIENT_SECRET`, then run `docker compose up --build`. PostgreSQL runs the
initial migration on first initialization of its volume.

## API

The OpenAPI 3.1 contract is at [openapi/openapi.yaml](openapi/openapi.yaml).

- `POST /v1/sms` submits and persists one message. A record is created as
  `queued` before calling Telbiz. Provider acceptance changes it to `sent`;
  confirmed delivery is not implied. An unconfirmed provider outcome remains
  `queued` with error detail to avoid falsely reporting failure.
- `GET /v1/sms` lists history, with `status`, `recipient`, `from`, `to`, `limit`
  (1–100, default 20), and `offset` filters. Time filters are inclusive RFC3339
  submission timestamps.
- `GET /v1/sms/{id}` retrieves a transaction.
- `POST /internal/v1/sms/{id}/delivery-status` accepts `delivered` or
  `undelivered` from a gateway-authenticated integration that translates a
  verified provider event. The Telbiz SDK documents no delivery query or
  webhook payload, so no provider callback format is assumed here.
- `GET /healthz` and `GET /readyz` provide liveness and database readiness.

SMS content, provider response details, and error detail are retained in the
transaction table and included in history responses. They are not written to
application logs. Restrict history access and retention through gateway policy
and database operations appropriate to the data.
