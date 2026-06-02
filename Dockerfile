# syntax=docker/dockerfile:1.6
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata \
    && echo 'nonroot:x:65532:65532:nonroot:/home/nonroot:/sbin/nologin' >> /etc/passwd \
    && echo 'nonroot:x:65532:' >> /etc/group
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/relayd ./cmd/relayd

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/luxfi/relay"
LABEL org.opencontainers.image.title="Lux Relay"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /etc/group /etc/group
COPY --from=build /out/relayd /usr/local/bin/relayd
ENV PORT=7700
EXPOSE 7700
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/relayd"]
