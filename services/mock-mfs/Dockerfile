FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY services/paylane-jwe /app/services/paylane-jwe
COPY services/mock-mfs /app/services/mock-mfs

WORKDIR /app/services/mock-mfs
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/mock-mfs ./cmd/mock-mfs

FROM alpine:3.20

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/bin/mock-mfs /app/mock-mfs
COPY services/mock-mfs/migrations /app/migrations

ENV PORT=5011
EXPOSE 5011

ENTRYPOINT ["/app/mock-mfs"]
