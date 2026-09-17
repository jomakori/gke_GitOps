FROM golang:1.24 AS build
ARG TARGETARCH
WORKDIR /src
COPY src/ .
RUN CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/gitopsctl ./cmd/gitopsctl

FROM scratch
COPY --from=build /out/gitopsctl /usr/local/bin/gitopsctl
ENTRYPOINT ["/usr/local/bin/gitopsctl"]
