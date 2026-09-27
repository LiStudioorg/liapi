// Types mirroring the liapi admin API JSON payloads (see server/handler_admin.go
// and config/config.go). Kept hand-written so the SPA has zero build coupling
// to the Go side.

export interface Overview {
  requests: number
  success: number
  client_errors: number
  upstream_errors: number
  latency_ms: number
  in_tokens: number
  out_tokens: number
  multimodal: number
  cost: number
  success_rate: number
  avg_latency_ms: number
  dropped_logs: number
  upstreams: number
  client_tokens: number
  devices: number
  models: number
  failovers: number
  cost_today: number
}

export interface Upstream {
  name: string
  base_url: string
  api_key?: string
  has_api_key?: boolean
  models: string[]
  priority: number
  weight: number
  disabled: boolean
  health_path?: string
  retry: number
  inject_usage: boolean
  group?: string
  model_map?: Record<string, string>
}

export interface ClientToken {
  masked: string
  full: string
}

export interface Device {
  id: string
  name?: string
  rpm: number
  daily: number
  disabled: boolean
  created_at?: string
  note?: string
  token_hint?: string
}

export interface DeviceCreateResult {
  ok: boolean
  device: Device
  token: string
}

export interface LogEntry {
  time: string
  request_id?: string
  token: string
  device?: string
  path: string
  model: string
  upstream: string
  status: number
  stream: boolean
  latency_ms: number
  in_tokens: number
  out_tokens: number
  multimodal?: number
  cost: number
  error?: string
}

export interface HealthStatus {
  name: string
  ok: boolean
  latency_ms: number
  last_check?: string
  error?: string
  consecutive_failures: number
}

export interface ProbeEvent {
  time: string
  name: string
  ok: boolean
  latency_ms: number
  error?: string
  manual?: boolean
}

export interface AuditEvent {
  time: string
  ip: string
  method: string
  path: string
  ok: boolean
  reason?: string
}

export interface StatRow {
  key: string
  requests: number
  success: number
  errors: number
  in_tokens: number
  out_tokens: number
  multimodal: number
  cost: number
  avg_latency_ms: number
  p50_ms: number
  p90_ms: number
  p99_ms: number
}

export interface StatsSnapshot {
  from: string
  to: string
  group_by: string
  total: StatRow
  rows: StatRow[]
  error_codes: Record<string, number>
}

export interface Price {
  input: number
  output: number
}

export interface TokenPolicy {
  expires_at?: string
  allow_ips?: string[]
  rpm?: number
}

export interface AlertConfig {
  webhooks?: string[]
  bark?: string[]
  on_failover: boolean
  on_unhealthy: boolean
  on_quota: boolean
  quota_warn_percent: number
  min_alert_interval_seconds: number
}

export interface AliasRule {
  pattern: string
  regex: boolean
  model: string
  params?: Record<string, unknown>
}

export interface UpstreamFull {
  name: string
  base_url: string
  api_key: string
  models: string[]
  priority: number
  weight: number
  disabled: boolean
  health_path: string
  retry: number
  inject_usage: boolean
  group?: string
  model_map?: Record<string, string>
}

export interface DeviceFull {
  id: string
  name?: string
  token_hash: string
  rpm: number
  daily: number
  disabled: boolean
  created_at?: string
  note?: string
}

export interface Config {
  addr: string
  body_limit_bytes: number
  timeout: number
  stream_timeout: number
  stream_idle_timeout: number
  max_idle_conns: number
  log_file: string
  log_max_bytes: number
  ring_size: number
  probe_interval: number
  probe_timeout: number
  probe_fail_threshold: number
  probe_concurrency: number
  health_state_file: string
  skip_unhealthy: boolean
  admin_token: string
  admin_rate_per_minute: number
  admin_allow_ips: string[]
  metrics_token: string
  client_tokens: string[]
  rate_limit_per_minute: number
  daily_per_token: number
  token_quotas: Record<string, number>
  token_policies: Record<string, TokenPolicy>
  alerts: AlertConfig
  log_retention_days: number
  log_raw_tokens: boolean
  aliases: Record<string, string>
  alias_rules: AliasRule[]
  prices: Record<string, Price>
  route_strategy: string
  fallbacks: Record<string, string[]>
  retry_on_timeout: boolean
  upstreams: UpstreamFull[]
  devices: DeviceFull[]
}

export interface TestResult {
  ok: boolean
  model: string
  upstream?: string
  status?: number
  body?: string
  error?: string
  usage?: { in_tokens: number; out_tokens: number }
  latency_ms: number
}

export interface ApiError {
  error?: {
    message?: string
    type?: string
    code?: string
  }
}
