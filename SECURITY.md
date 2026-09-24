# Security Policy

## Supported Versions

Only the latest release / `main` branch of liapi receives fixes. Older binaries should be upgraded.

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Report privately via one of:

- GitHub Security Advisories for this repository (**Security → Report a vulnerability**), preferred.
- Email the maintainer(s) listed on the organization profile: https://github.com/LiStudioorg

Include:

1. Affected version / commit
2. Reproduction steps or a proof of concept
3. Impact (e.g. auth bypass, secret leak, RCE)

You should receive an acknowledgement within 7 days. Please allow reasonable time for a fix before disclosure.

## Security Notes for Operators

liapi stores secrets in `config.json` (admin token, client tokens, upstream API keys, device token hashes).

- The file **must be mode `0600`**. `liapi` refuses to start otherwise (bypass: `LIAPI_SKIP_PERM_CHECK=1`, only for recovery).
- Device tokens are stored **only as SHA-256 hashes**; plaintext is shown once at creation/rotation.
- The admin API supports per-IP rate limiting (`admin_rate_per_minute`) and an optional IP allowlist (`admin_allow_ips`).
- `/metrics` is protected by `metrics_token` (or the admin token when unset). Set `"metrics_token": "-"` only on trusted networks.
- Tokens are masked in logs (`sk-ab...xyz`); upstream API keys are never logged.
- Stream/relay traffic uses constant-time token comparison (`crypto/subtle`) to avoid timing attacks.

## Out of Scope

- Denial of service against upstream LLM providers
- Issues requiring a compromised host with filesystem access to `config.json`
