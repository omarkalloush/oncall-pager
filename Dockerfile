# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install git and root certificates
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod ./

# We let 'go mod tidy' run in the container to generate go.sum
COPY . .
RUN go mod tidy

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /oncall-pager .

# Final stage
FROM alpine:latest

# Import certs and timezone data from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy our static executable
COPY --from=builder /oncall-pager /oncall-pager

# Run the binary
ENTRYPOINT ["/oncall-pager"]
