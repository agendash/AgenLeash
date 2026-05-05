FROM golang:1.26.2-bookworm AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY adapters ./adapters
COPY cmd ./cmd
COPY docs ./docs
COPY internal ./internal
COPY .env.example README.md ./

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/agenleash ./cmd/agenleash

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates curl git openssh-client tini \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /var/lib/agenleash

COPY --from=builder /out/agenleash /usr/local/bin/agenleash
COPY adapters /usr/local/share/agenleash/adapters
COPY .env.example /usr/local/share/agenleash/.env.example
COPY packaging/systemd/agenleash.service /usr/local/share/agenleash/examples/agenleash.service
COPY packaging/launchd/io.agenleash.plist /usr/local/share/agenleash/examples/io.agenleash.plist

ENV AGENLEASH_ADDR=0.0.0.0:8081 \
    AGENLEASH_ADAPTER_DIR=/usr/local/share/agenleash/adapters \
    AGENLEASH_DATA_DIR=/var/lib/agenleash

EXPOSE 8081

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/agenleash"]
