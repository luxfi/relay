# syntax=docker/dockerfile:1.6
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/relayd ./cmd/relayd

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/luxfi/relay"
LABEL org.opencontainers.image.title="Lux Relay"
COPY --from=build /out/relayd /usr/local/bin/relayd
ENV PORT=7700
EXPOSE 7700
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/relayd"]
