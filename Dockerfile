# ---- build ----
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/liapi .

# ---- run ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H liapi
COPY --from=build /out/liapi /usr/local/bin/liapi
# config.json (0600) and relay.jsonl live here; mount a volume over /data
RUN mkdir -p /data && chown liapi:liapi /data
WORKDIR /data
USER liapi
EXPOSE 8787
ENV LIAPI_SKIP_PERM_CHECK=
ENTRYPOINT ["/usr/local/bin/liapi"]
CMD ["-config", "/data/config.json"]
