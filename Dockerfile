FROM golang:1.26 AS builder

WORKDIR /src

COPY src/go.mod src/go.sum ./
RUN go mod download

COPY src/main.go .
COPY src/controller.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server .

RUN GOBIN=/out CGO_ENABLED=0 go install github.com/b-zago/rikami@v0.1.0 && \
  GOBIN=/out CGO_ENABLED=0 go install github.com/bitnami-labs/sealed-secrets/cmd/kubeseal@v0.36.6

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY src/conf /usr/local/bin/conf 
COPY --from=builder /out/server /usr/local/bin/server
COPY --from=builder /out/rikami /usr/local/bin/rikami
COPY --from=builder /out/kubeseal /usr/local/bin/kubeseal

EXPOSE 8080
CMD ["server"]
