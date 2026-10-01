# ---- frontend (Nuxt admin SPA) ----
FROM node:22-alpine AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
# Outputs to ../server/adminui relative to web/
RUN npm run build

# ---- build ----
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download 2>/dev/null || true
COPY . .
# Overlay the freshly built admin SPA over whatever is committed.
COPY --from=frontend /server/adminui ./server/adminui
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
