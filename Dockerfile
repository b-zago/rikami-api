FROM golang:1.26.4-alpine3.24 AS builder
WORKDIR /out
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/server /usr/local/bin/server

EXPOSE 8080
CMD ["server"]
