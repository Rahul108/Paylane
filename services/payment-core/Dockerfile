FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy shared infrastructure library
COPY services/paylane-jwe /app/services/paylane-jwe

# Copy service source
COPY services/payment-core /app/services/payment-core

WORKDIR /app/services/payment-core
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/payment-core ./cmd/payment-core

FROM alpine:3.20

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/bin/payment-core /app/payment-core
COPY services/payment-core/migrations /app/migrations

ENV PORT=4011
EXPOSE 4011

ENTRYPOINT ["/app/payment-core"]
