FROM golang:1.23-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /openagent-discord-bot .

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /openagent-discord-bot /openagent-discord-bot
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/openagent-discord-bot"]
