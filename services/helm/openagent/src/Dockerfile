FROM golang:1.23-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates curl
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /openagent-discord-bot .

ARG RG_VERSION=15.1.0
ARG TARGETARCH
RUN if [ "${TARGETARCH}" = "arm64" ]; then \
      RG_ARCH="aarch64-unknown-linux-gnu"; \
    else \
      RG_ARCH="x86_64-unknown-linux-musl"; \
    fi && \
    curl -fsSL https://github.com/BurntSushi/ripgrep/releases/download/${RG_VERSION}/ripgrep-${RG_VERSION}-${RG_ARCH}.tar.gz \
    | tar xz -C /tmp && cp /tmp/ripgrep-${RG_VERSION}-${RG_ARCH}/rg /rg

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /openagent-discord-bot /openagent-discord-bot
COPY --from=builder /rg /usr/local/bin/rg
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/openagent-discord-bot"]
