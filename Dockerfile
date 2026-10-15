# ---- build ----
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Pure-Go SQLite driver -> fully static binaries, no CGO.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/driftctl ./cmd/driftctl \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/drift-server ./cmd/drift-server

# ---- runtime ----
FROM alpine:3.20
# alpine already ships CA certificates and busybox wget (used by HEALTHCHECK).
RUN adduser -D -u 10001 drift \
 && mkdir -p /data && chown drift /data
WORKDIR /app
COPY --from=build /out/ /usr/local/bin/
COPY configs/ /app/configs/
USER drift
ENV DRIFTCTL_DB=/data/driftctl.db
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s CMD wget -qO- http://localhost:8080/health || exit 1
ENTRYPOINT ["drift-server"]
CMD ["-config", "/app/configs/driftctl.yaml"]
