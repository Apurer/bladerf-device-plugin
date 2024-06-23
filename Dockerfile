# Build stage using the golang base image
FROM golang:buster as builder

# Define working directory.
WORKDIR /opt/bladerf-device-plugin

# Copy the source code into the container
COPY . .

# Install build dependencies and build the binary
RUN apt-get update && \
    apt-get install -d -y git && \
    go get -d ./. && \
    go build -o bin/bladerf-device-plugin

# Final stage, use Ubuntu 20.04
# Install essential tools and add the PPA
FROM ubuntu:20.04

# Install essential tools and add the PPA


RUN apt-get update && \
    apt-get install -y software-properties-common && \
    add-apt-repository -y ppa:nuandllc/bladerf && \
    apt-get update

# Install the BladeRF software and development libraries
RUN apt-get install -y bladerf libbladerf-dev

RUN apt-get install -y lsof usbutils

# Set the working directory in the container
WORKDIR /usr/local/bin

# Copy the binary from the builder stage
COPY --from=builder /opt/bladerf-device-plugin/bin/bladerf-device-plugin .

# Set the container's entrypoint to the binary
ENTRYPOINT ["/usr/local/bin/bladerf-device-plugin"]
