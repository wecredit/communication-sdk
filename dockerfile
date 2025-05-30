# ------------ Multi-Stage Dockerfile for API Build ------------

# Stage 1: Build Stage
FROM golang:1.23.2 AS builder

# Set the working directory inside the builder container
WORKDIR /go/src/communication-sdk
# Copy the entire Go project into the builder container
COPY . .

# # Copy go.mod and go.sum files to the working directory
# COPY go.mod go.sum ./

RUN ls -l

# Download all Go dependencies
RUN go mod download

# Build the Go application (main.go located in /cmd/api/main.go)
RUN CGO_ENABLED=0 GOOS=linux go build -o /go/bin/main /go/src/communication-sdk/cmd/consumerLayer/main.go

# Stage 2: Run Stage
FROM alpine:latest

# Install necessary certificates for HTTPS (if required by your application)
RUN apk add --no-cache ca-certificates tzdata

# Set timezone to Asia/Kolkata
ENV TZ=Asia/Kolkata

# Verify the timezone file exists
RUN ls -l /usr/share/zoneinfo/Asia/Kolkata

# Set the working directory in the final container
WORKDIR /app

# Copy the compiled Go binary from the builder stage
COPY --from=builder /go/bin/main .

# Expose the port your application uses
EXPOSE 8080

# Command to run the Go application
CMD ["./main"]
 