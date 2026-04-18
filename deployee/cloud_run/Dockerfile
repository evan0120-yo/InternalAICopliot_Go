# Build context: InternalAICopliot/Backend/Go/
# Usage: docker build -f deployee/cloud_run/Dockerfile .
# Serves HTTP + gRPC on the same PORT via h2c (HTTP/2 cleartext).

FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o internal-ai-server ./cmd/api

# ---- runtime ----
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/internal-ai-server .

EXPOSE 8080
CMD ["./internal-ai-server"]
