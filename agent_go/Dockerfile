# Build stage
FROM golang:1.24-alpine AS builder

# Install git, C compiler, and other dependencies
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o mcp-agent .

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests, Node.js for MCP servers, C libraries for CGO, Python for uvx, jq for JSON processing, and ripgrep for text searching
RUN apk --no-cache add ca-certificates nodejs npm gcc musl-dev python3 py3-pip jq ripgrep

# Install uvx (uv package manager) with system packages override
RUN pip3 install --break-system-packages uv

# Verify npx is available and add to PATH if needed
RUN which npx || echo "npx not found, checking npm location"
RUN npm --version && npx --version

# Verify ripgrep and jq are available
RUN which rg && which jq
RUN rg --version && jq --version

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -S appuser -u 1001 -G appgroup

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/mcp-agent .

# Copy configuration files
COPY --from=builder /app/configs ./configs

# Create reports and logs directories
RUN mkdir -p reports logs && chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8000

# Set default command
CMD ["./mcp-agent", "server", "--host", "0.0.0.0", "--port", "8000", "--log-file", "/app/logs/server_debug.log", "--log-level", "debug", "--mcp-config", "configs/mcp_servers_simple.json"] 