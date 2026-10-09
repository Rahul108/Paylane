FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY services/paylane-jwe /app/services/paylane-jwe
COPY services/mock-downstream /app/services/mock-downstream

WORKDIR /app/services/mock-downstream
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/mock-downstream ./cmd/mock-downstream

FROM alpine:3.20

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/bin/mock-downstream /app/mock-downstream
COPY services/mock-downstream/migrations /app/migrations

ENV PORT=5013
EXPOSE 5013

ENTRYPOINT ["/app/mock-downstream"]
