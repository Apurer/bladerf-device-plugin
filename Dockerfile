# Build stage
FROM golang:buster as builder

# Define working directory.
WORKDIR /opt/bladerf-device-plugin

# Copy the source code into the container
COPY . .

# Install dependencies and build the binary
RUN go get -d ./. && \
    go build -o bin/bladerf-device-plugin

# Final stage
FROM debian:buster-slim

# Set the working directory in the container
WORKDIR /usr/local/bin

# Copy the binary from the builder stage
COPY --from=builder /opt/bladerf-device-plugin/bin/bladerf-device-plugin .

# Set the container's entrypoint to the binary
ENTRYPOINT ["/usr/local/bin/bladerf-device-plugin"]
