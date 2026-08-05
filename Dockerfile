# syntax=docker/dockerfile:1

# golang:1.25 matches go.mod's `go 1.25` minimum — bump both together.
FROM golang:1.25-bookworm AS builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY cmd/ cmd/
COPY internal/ internal/

# TARGETOS/TARGETARCH are set by buildx per platform, so one stage cross-compiles both.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o querier ./cmd/querier

# Stage 2: Create the final image
FROM alpine:3.24

ENV PUID=1000
ENV PGID=1000

RUN addgroup -g ${PGID} appgroup && \
    adduser -u ${PUID} -G appgroup -s /bin/sh -D appuser

WORKDIR /usr/src/app

COPY --from=builder /usr/src/app/querier .
COPY --from=builder /usr/src/app/internal/graph /usr/src/app/internal/graph

USER appuser

EXPOSE 8080
EXPOSE 5353/udp

CMD ["./querier"]
