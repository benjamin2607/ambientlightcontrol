# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/ambientlightd ./cmd/ambientlightd \
 && mkdir -p /out/config

# Minimal runtime: no shell, no package manager, only the binary and the CA certificates.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/ambientlightd /ambientlightd
# 99:100 is nobody:users, the owner of Unraid's appdata shares.
COPY --from=build --chown=99:100 /out/config /config
USER 99:100
ENV CONFIG_DIR=/config \
    LISTEN_ADDR=:8787
EXPOSE 8787
VOLUME /config
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/ambientlightd", "healthcheck"]
ENTRYPOINT ["/ambientlightd"]
