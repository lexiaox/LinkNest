FROM golang:1.22-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY server ./server
COPY deploy ./deploy

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/linknest-server ./server/cmd/linknest-server

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/linknest-server /app/linknest-server
COPY server /app/server
COPY deploy /app/deploy

RUN mkdir -p /var/lib/linknest/storage /var/lib/linknest/chunks

EXPOSE 8080

CMD ["/app/linknest-server", "--config", "/app/deploy/config.docker.yaml"]
