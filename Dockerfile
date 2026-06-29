FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine3.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /out
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o server .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/server . 
COPY --from=builder /out/errors.json .

EXPOSE 8080
CMD ["./server"]
