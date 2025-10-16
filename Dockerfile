# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o icyproxy ./cmd/icyproxy

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/icyproxy .
COPY configs/example.yaml /etc/icyproxy/config.yaml

EXPOSE 8000

CMD ["./icyproxy", "/etc/icyproxy/config.yaml"]
