/**
 * 真实 API 客户端 —— 全部走 fetch，无任何 Mock 数据。
 * 默认同源 /api；开发环境用 Vite 代理转发到后端 3002。
 * 也可用 VITE_API_BASE 指向其它地址（例如手机访问局域网 IP）。
 */
const BASE = import.meta.env.VITE_API_BASE ?? "";

export type Device = {
  id: string;
  name: string;
  token_masked: string;
  rpm: number | null;
  status: "normal" | "disabled";
  month_cost: number;
  created_at: string;
  updated_at: string;
  /** 仅在创建 / 轮换时返回一次 */
  token?: string;
};

export type LogRow = {
  id: number;
  device_id: string | null;
  model: string | null;
  upstream: string | null;
  status: number | null;
  in_tokens: number;
  out_tokens: number;
  latency_ms: number;
  error: string | null;
  created_at: string;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });
  const text = await res.text();
  const data = text ? (() => { try { return JSON.parse(text); } catch { return text; } })() : null;
  if (!res.ok) {
    const msg = (data && typeof data === "object" && "error" in data
      ? (data as any).error
      : typeof data === "string" ? data : `请求失败 (${res.status})`) as string;
    throw new Error(msg);
  }
  return data as T;
}

export type Upstream = {
  id: string;
  name: string;
  protocol: "OpenAI" | "Anthropic";
  api_key_masked: string;
  has_key: boolean;
  status: "normal" | "error";
  enabled: boolean;
  models: number;
};

export type ModelRow = {
  id: string;
  upstream: string;
  display: string;
  alias: string;
  caps: string[];
  upstream_id: string | null;
  upstream_name: string | null;
};

export type AuditRow = {
  id: number;
  actor: string;
  action: string;
  target: string | null;
  ip: string | null;
  ok: number;
  created_at: string;
};

export type Settings = Record<string, any>;

export type StatsSummary = {
  calls: number;
  in_tokens: number;
  out_tokens: number;
  tokens: number;
  cost: number;
  avg_latency: number;
  errors: number;
};

export type UsageBucket = {
  bucket: string;
  upstream: string;
  model: string;
  calls: number;
  in_tokens: number;
  out_tokens: number;
  cost: number;
};

export type DailyPoint = { day: string; calls: number; tokens: number; avg_latency: number };

export type ModelStat = { model: string; calls: number; tokens: number; avg_latency: number };

export const api = {
  listTokens: () => req<Device[]>("/api/tokens"),

  createToken: (name: string, rpm: number | null) =>
    req<Device>("/api/tokens", {
      method: "POST",
      body: JSON.stringify({ name, rpm }),
    }),

  updateToken: (id: string, patch: { name?: string; rpm?: number | null; status?: string }) =>
    req<Device>(`/api/tokens/${id}`, { method: "PUT", body: JSON.stringify(patch) }),

  deleteToken: (id: string) =>
    req<{ ok: boolean; id: string }>(`/api/tokens/${id}`, { method: "DELETE" }),

  rotateToken: (id: string) =>
    req<Device>(`/api/tokens/${id}/rotate`, { method: "POST", body: "{}" }),

  listLogs: (n = 20) => req<LogRow[]>(`/api/logs?n=${n}`),

  overview: () => req<{ calls: number; tokens: number; devices: number }>("/api/overview"),

  /* ── 上游 ── */
  listUpstreams: () => req<Upstream[]>("/api/upstreams"),
  createUpstream: (b: { name: string; protocol: string; api_key?: string }) =>
    req<Upstream>("/api/upstreams", { method: "POST", body: JSON.stringify(b) }),
  updateUpstream: (id: string, b: Record<string, any>) =>
    req<Upstream>(`/api/upstreams/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  deleteUpstream: (id: string) => req<{ ok: boolean }>(`/api/upstreams/${id}`, { method: "DELETE" }),
  probeUpstream: (id: string) =>
    req<{ id: string; status: string; latency_ms: number; http_status?: number; error?: string }>(
      `/api/upstreams/${id}/probe`,
      { method: "POST", body: "{}" }
    ),

  /* ── 模型 ── */
  listModels: () => req<ModelRow[]>("/api/models"),
  createModel: (b: Record<string, any>) =>
    req<ModelRow>("/api/models", { method: "POST", body: JSON.stringify(b) }),
  updateModel: (id: string, b: Record<string, any>) =>
    req<ModelRow>(`/api/models/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  deleteModel: (id: string) => req<{ ok: boolean }>(`/api/models/${id}`, { method: "DELETE" }),

  /* ── 设置 ── */
  getSettings: () => req<Settings>("/api/settings"),
  saveSettings: (b: Record<string, any>) =>
    req<Settings>("/api/settings", { method: "PUT", body: JSON.stringify(b) }),
  clearCache: () => req<{ ok: boolean }>("/api/settings/cache/clear", { method: "POST", body: "{}" }),
  testWebhook: (url: string) =>
    req<{ ok: boolean; http_status?: number }>("/api/settings/webhook/test", {
      method: "POST",
      body: JSON.stringify({ url }),
    }),

  /* ── 审计与统计 ── */
  listAudit: (n = 50) => req<AuditRow[]>(`/api/audit?n=${n}`),
  statsSummary: (hours = 24) => req<StatsSummary>(`/api/stats/summary?hours=${hours}`),
  statsUsage: (hours = 24, model = "all", source = "all") =>
    req<UsageBucket[]>(`/api/stats/usage?hours=${hours}&model=${encodeURIComponent(model)}&source=${encodeURIComponent(source)}`),
  statsDaily: (days = 7) => req<DailyPoint[]>(`/api/stats/daily?days=${days}`),
  statsModels: () => req<ModelStat[]>("/api/stats/models"),
};
