FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY config.example.yaml VERSION ./
RUN version="${VERSION:-$(cat VERSION)}" && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$version" -o /evigo ./cmd/evigo

FROM builder AS test
RUN apk add --no-cache gcc musl-dev
RUN go vet ./cmd/... ./internal/... && go test -race ./cmd/... ./internal/...

FROM alpine:3.24
RUN apk add --no-cache iputils ca-certificates && \
    addgroup -g 10001 evigo && adduser -D -H -u 10001 -G evigo evigo && \
    mkdir -p /var/lib/evigo /etc/evigo && chown evigo:evigo /var/lib/evigo
COPY --from=builder /evigo /usr/local/bin/evigo
COPY LICENSE THIRD_PARTY_LICENSES /usr/share/licenses/evigo/
USER 10001:10001
ENTRYPOINT ["evigo"]
CMD ["run", "--config", "/etc/evigo/config.yaml"]
