# Stage 1: Build the binary
FROM golang:1.23.0-alpine AS builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -o querier cmd/querier/main.go

# Stage 2: Create the final image
FROM alpine:3.20

# Set environment variables for user and group IDs
ENV PUID=1000
ENV PGID=1000

# Create a group and user with the specified GID and UID
RUN addgroup -g ${PGID} appgroup && \
    adduser -u ${PUID} -G appgroup -s /bin/sh -D appuser

WORKDIR /usr/src/app

# Copy the compiled binary from the builder stage
COPY --from=builder /usr/src/app/querier .

# Copy the static files
COPY --from=builder /usr/src/app/internal/graph /usr/src/app/internal/graph

# Set the user
USER appuser

EXPOSE 8080      
EXPOSE 5353/udp

CMD ["./querier"]
