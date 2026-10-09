FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY services/paylane-jwe /app/services/paylane-jwe
COPY services/orchestrator /app/services/orchestrator

WORKDIR /app/services/orchestrator
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/orchestrator ./cmd/orchestrator

FROM alpine:3.20

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/bin/orchestrator /app/orchestrator
COPY services/orchestrator/migrations /app/migrations

ENV PORT=4010
EXPOSE 4010

ENTRYPOINT ["/app/orchestrator"]
