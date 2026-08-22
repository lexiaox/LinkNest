FROM golang:1.22-bookworm AS builder

ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY server ./server
COPY deploy ./deploy

RUN go test ./server/...
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/linknest-server ./server/cmd/linknest-server

FROM debian:bookworm-slim

RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g; s|security.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/linknest-server /app/linknest-server
COPY server /app/server
COPY deploy /app/deploy

RUN mkdir -p /var/lib/linknest/storage /var/lib/linknest/chunks

EXPOSE 8080

CMD ["/app/linknest-server", "--config", "/app/deploy/config.docker.yaml"]
