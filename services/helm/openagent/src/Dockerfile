FROM golang:1.23-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates curl
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /openagent-discord-bot .

ARG RG_VERSION=15.1.0
RUN curl -fsSL https://github.com/BurntSushi/ripgrep/releases/download/${RG_VERSION}/ripgrep-${RG_VERSION}-x86_64-unknown-linux-musl.tar.gz \
    | tar xz -C /tmp && cp /tmp/ripgrep-${RG_VERSION}-x86_64-unknown-linux-musl/rg /rg

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /openagent-discord-bot /openagent-discord-bot
COPY --from=builder /rg /usr/local/bin/rg
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/openagent-discord-bot"]
