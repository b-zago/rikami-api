FROM golang:1.26 AS builder

WORKDIR /src

COPY src/go.mod src/go.sum ./
RUN go mod download

COPY src/main.go .
COPY src/controller.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server .

FROM debian:bookworm-slim

ARG TARGETARCH
ARG KUBESEAL_VER=v0.36.6
ARG RIKA_VER=v0.1.3

RUN apt-get update && apt-get install -y --no-install-recommends curl git ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN if [ "$TARGETARCH" = "amd64" ]; then \
  KARCH="amd64"; RARCH="x86_64"; \
  elif [ "$TARGETARCH" = "arm64" ]; then \
  KARCH="arm64"; RARCH="arm64"; \
  else \
  echo "unsupported arch: $TARGETARCH" && exit 1; \
  fi && \
  curl -sL "https://github.com/bitnami-labs/sealed-secrets/releases/download/${KUBESEAL_VER}/kubeseal-${KUBESEAL_VER#v}-linux-${KARCH}.tar.gz" | tar xz -C /usr/local/bin kubeseal && \
  curl -sL "https://github.com/b-zago/rikami/releases/download/${RIKA_VER}/rikami_Linux_${RARCH}.tar.gz" | tar xz -C /usr/local/bin rika

WORKDIR /app
COPY src/conf .

COPY --from=builder /out/server /usr/local/bin/server

EXPOSE 8080
CMD ["server"]
