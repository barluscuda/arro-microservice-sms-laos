FROM golang:1.27.1-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sms-api ./cmd/sms-api

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/sms-api /app/sms-api
COPY migrations /app/migrations
USER app
EXPOSE 3285
ENTRYPOINT ["/app/sms-api"]
