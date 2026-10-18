# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26


FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X velexa-sim-me/internal/config.Version=${VERSION}" \
      -o /out/velexa-sim-me .
FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
 && adduser -D -H -u 10001 velexa \
 && mkdir -p /data && chown 10001:10001 /data
WORKDIR /app
COPY --from=build /out/velexa-sim-me /app/velexa-sim-me
COPY spec /app/spec
ENV ME_STATE_DIR=/data \
    ME_ENV_FILE=/data/me.env \
    ME_ENV_GENERATE=off \
    ME_FIX_SPEC_DIR=/app/spec \
    ME_INSTRUMENT_FILE=/data/instruments.json \
    ME_IN_CONTAINER=1 \
    TZ=Asia/Dhaka \
    PORT=8010 FIX_PORT=5001 ITCH_PORT=5501 ME_ITCH_INDEX_PORT=5502 ME_ITCH_NEWS_PORT=5503

USER 10001:10001
VOLUME ["/data"]
EXPOSE 8010 5001 5501 5502 5503
HEALTHCHECK --interval=15s --timeout=5s --start-period=40s --retries=3 \
  CMD ["/app/velexa-sim-me", "healthcheck"]
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/velexa-sim-me"]
