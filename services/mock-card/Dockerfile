FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY services/paylane-jwe /app/services/paylane-jwe
COPY services/mock-card /app/services/mock-card

WORKDIR /app/services/mock-card
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/mock-card ./cmd/mock-card

FROM alpine:3.20

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/bin/mock-card /app/mock-card
COPY services/mock-card/migrations /app/migrations

ENV PORT=5012
EXPOSE 5012

ENTRYPOINT ["/app/mock-card"]
