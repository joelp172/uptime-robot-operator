#syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.6.1 AS xx

FROM --platform=$BUILDPLATFORM golang:1.24.13-alpine AS builder
WORKDIR /app

COPY --from=xx / /

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

ARG TARGETPLATFORM
RUN --mount=type=cache,target=/root/.cache \
  CGO_ENABLED=0 xx-go build -ldflags='-w -s' -tags grpcnotrace -trimpath -o manager cmd/main.go


FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /app/manager /

ENTRYPOINT ["/manager"]
