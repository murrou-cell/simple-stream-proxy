# ============================
# 1. Build Stage
# ============================
FROM golang:1.23.1-alpine AS builder

WORKDIR /app

# Install build deps
RUN apk add --no-cache git


COPY . .
RUN go mod download

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -o proxy .

# ============================
# 2. Runtime Stage
# ============================
FROM alpine:latest

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/proxy .

# Run the proxy
CMD ["./proxy"]
