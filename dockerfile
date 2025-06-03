# ------------ Multi-Stage Dockerfile for API Build ------------

# Stage 1: Build Stage
FROM golang:1.23.4 AS builder

# Set working directory inside the builder container
WORKDIR /go/src/communication-sdk

# Copy go.mod and go.sum first to leverage Docker layer caching
COPY go.mod go.sum ./

# Download Go module dependencies
RUN go mod download

# Copy the rest of the source code
COPY . .

# Optional: Debugging help - check the presence of main.go
RUN ls -alh /go/src/communication-sdk/cmd/consumerLayer/

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /go/bin/main ./cmd/consumerLayer/main.go

# Stage 2: Run Stage
FROM alpine:latest

# Install certificates and timezone info
RUN apk add --no-cache ca-certificates tzdata

# Set timezone
ENV TZ=Asia/Kolkata

# Set the working directory
WORKDIR /app

# Copy compiled binary from builder stage
COPY --from=builder /go/bin/main .

# Expose the application port
EXPOSE 8080

# Run the application
CMD ["./main"]
