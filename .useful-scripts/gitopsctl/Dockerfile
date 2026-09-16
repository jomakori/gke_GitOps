# gitopsctl ships as one static binary. Its only consumer in the cluster is an
# init container that runs `cp -f`, so the runtime layer needs the binary plus a
# shell-free copy tool — busybox gives `cp` in ~1 MB.
#
# Build context is this directory (.useful-scripts/gitopsctl); see
# .github/workflows/gitopsctl.yaml. Stdlib-only: there is no go.sum and no
# module download step.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -trimpath -ldflags="-s -w" -o /out/gitopsctl ./cmd/gitopsctl

FROM busybox:1.38-musl
COPY --from=build /out/gitopsctl /usr/local/bin/gitopsctl
# `docker run <image> ...` behaves like the CLI, which is what you want when
# debugging this image by hand.
ENTRYPOINT ["/usr/local/bin/gitopsctl"]
