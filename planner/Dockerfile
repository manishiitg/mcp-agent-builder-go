# Build stage
FROM golang:1.24-alpine AS builder

# Install ripgrep, gcc, and musl-dev for CGO support
RUN apk add --no-cache ripgrep gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled for SQLite
ENV CGO_ENABLED=1
RUN go build -o planner .

# Final stage
FROM alpine:latest

# Install ripgrep, git, and patch for GitHub integration and diff patching
RUN apk add --no-cache ripgrep git patch

# Create app user
RUN adduser -D -s /bin/sh appuser

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/planner .

# Create docs and data directories
RUN mkdir -p /app/planner-docs /app/data && chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./planner", "server", "--port", "8080", "--docs-dir", "/app/planner-docs"]
