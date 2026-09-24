# Contributing to liapi

Thanks for helping improve liapi — a self-hosted, OpenAI-compatible API gateway written in **pure Go standard library** (no third-party module dependencies).

## Ground Rules

1. **Zero third-party dependencies.** `go.mod` must stay stdlib-only. If you need functionality, implement it in-tree or make a convincing case in an issue first.
2. **Never log secrets.** Tokens/API keys must be masked (`common.MaskToken`) or omitted. Device tokens exist only as SHA-256 hashes at rest.
3. **Keep the boundary.** liapi is a gateway: it does not run model inference and is not a multi-tenant SaaS.

## Getting Started

```bash
git clone https://github.com/LiStudioorg/liapi.git
cd liapi
go test ./...
go build -o liapi .
./liapi -config config.json   # first run generates config.json (mode 0600)
```

Open the admin UI at `http://localhost:8787/` (admin token is printed on first start, masked thereafter — keep the file).

## Before You Submit a PR

All of these must pass locally (they also run in CI):

```bash
gofmt -l .          # must print nothing
go vet ./...
go test ./...
go build ./...
```

### Tests

- Put tests next to the code: `foo.go` → `foo_test.go`.
- Core logic that must be covered: routing (fallback chains, strategies, aliases), retry/failover classification, limiter/quota windows, config validation, device auth.
- Prefer `httptest` for relay/HTTP tests; inject a fake clock (`SetNow`) for time-based logic instead of `time.Sleep`.

### PR Guidelines

- One concern per PR; include motivation and a short test plan in the description.
- Config/schema changes: update the field table in `README.md` and mention migration impact.
- Do not bump `go` directive without discussion.

## Release

Maintainers tag `vX.Y.Z`; `.github/workflows/release.yml` cross-compiles via `buildrelease.sh` (version injected into `main.version`).

## Code Map

| Package | Responsibility |
|---|---|
| `config` | Schema, validation, atomic save, hot-reload holder, OneAPI interop |
| `auth` | Client/device/admin authentication |
| `routing` | Candidate selection: fallback chains, priority/latency/cost, aliases |
| `relay` | Forwarding, retry/failover classification, SSE copy, usage extraction |
| `server` | HTTP handlers, request-id, admin API, metrics |
| `stats` | Sliding-window limiter, daily quota, JSONL logger, health probes, metrics, aggregation |
| `common` | Errors (OpenAI format), token hashing/masking |

## Reporting Bugs / Security

- Bugs: GitHub issues with repro steps.
- Security: see [SECURITY.md](SECURITY.md) — do **not** file public issues for vulnerabilities.
