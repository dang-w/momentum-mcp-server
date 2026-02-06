# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Download dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 go build -o momentum-server .

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/momentum-server .
EXPOSE 8080
CMD ["./momentum-server"]
