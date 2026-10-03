# syntax=docker/dockerfile:1.7

FROM golang:1.27.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY content ./content

RUN CGO_ENABLED=0 go build \
      -trimpath \
      -buildvcs=false \
      -ldflags='-s -w' \
      -o /out/api \
      ./cmd/app
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/bootstrap ./cmd/bootstrap

FROM alpine:3.22 AS runtime

RUN addgroup -S app && adduser -S -G app app

RUN apk add --no-cache ca-certificates
COPY --from=certs russian_trusted_root_ca.crt russian_trusted_sub_ca.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates

WORKDIR /app
COPY --from=builder /out/api /app/api
COPY --from=builder /out/bootstrap /app/bootstrap
COPY --from=builder /src/content /app/content
COPY --from=openapi openapi.yaml /app/openapi.yaml

USER app

ENTRYPOINT ["/app/api"]
