FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download 2>/dev/null || true
COPY cmd/plugin/ ./cmd/plugin/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /plugin ./cmd/plugin/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /plugin /usr/local/bin/plugin-gitea-wiki
ENTRYPOINT ["/usr/local/bin/plugin-gitea-wiki"]
