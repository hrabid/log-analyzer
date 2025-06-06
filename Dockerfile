# Multi-stage build Dockerfile for Log Analyzer
FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o loganalyzer .

# Final stage - minimal image
FROM alpine:latest

# Install ca-certificates for HTTPS requests if needed
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S loganalyzer && \
    adduser -u 1001 -S loganalyzer -G loganalyzer

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/loganalyzer .

# Create directories for logs
RUN mkdir -p /logs && \
    chown -R loganalyzer:loganalyzer /app /logs

# Switch to non-root user
USER loganalyzer

# Set default command
ENTRYPOINT ["./loganalyzer"]

# Default help command
CMD ["--help"]

# Metadata
LABEL maintainer="your-email@example.com"
LABEL description="Log Analyzer CLI Tool"
LABEL version="1.0"

# Document exposed volumes
VOLUME ["/logs"]