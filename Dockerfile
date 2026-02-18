# Copyright 2026 The Butler Authors.
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /butler-runner .

FROM alpine:3.20

RUN apk add --no-cache git curl unzip gnupg ca-certificates && \
    adduser -D -u 65534 runner

COPY --from=builder /butler-runner /usr/local/bin/butler-runner

USER runner
WORKDIR /workspace

ENTRYPOINT ["butler-runner"]
CMD ["exec"]
