export type Provider = {
  id: string;
  name: string; // 上游地址，如 https://zc.gcmod.cn/v1
  protocol: "OpenAI" | "Anthropic";
  status: "normal" | "error" | "probing";
  latencyMs: number | null;
  models: number;
};

export type ModelRow = {
  id: string;
  upstream: string; // 上游模型名
  caps: ("视觉" | "工具调用" | "深度思考" | "128K+上下文")[];
  display: string; // 显示名称
  alias: string; // 客户端别名
  providerId: string;
};

export type Device = {
  id: string;
  name: string;
  tokenPrefix: string;
  rpm: number | null; // null = 无限制
  status: "normal" | "disabled";
  monthCost: number;
};

export type LogRow = {
  id: string;
  time: string;
  status: number;
  model: string;
  upstream: string;
  tokens: number;
  latencyMs: number;
  source: string;
};

export type UsageRow = {
  hour: string;
  provider: string;
  model: string;
  req: number;
  inTok: number;
  outTok: number;
  cost: number;
};

export type AuditRow = {
  time: string;
  actor: string;
  action: string;
  target: string;
  ip: string;
  result: "成功" | "失败";
};

export type Snapshot = {
  version: string;
  time: string;
  author: string;
  summary: string;
};

export const BASE_URL = "https://cpg.z321.cc.cd";

export const providers: Provider[] = [
  { id: "p1", name: "https://zc.gcmod.cn/v1", protocol: "OpenAI", status: "normal", latencyMs: 812, models: 6 },
  { id: "p2", name: "https://api.openai.com/v1", protocol: "OpenAI", status: "normal", latencyMs: 1240, models: 4 },
  { id: "p3", name: "https://api.anthropic.com/v1", protocol: "Anthropic", status: "error", latencyMs: null, models: 3 },
  { id: "p4", name: "https://oneapi.example.net/v1", protocol: "OpenAI", status: "normal", latencyMs: 460, models: 2 },
];

export const models: ModelRow[] = [
  { id: "m1", upstream: "gpt-5.6-luna", caps: ["视觉", "工具调用", "深度思考", "128K+上下文"], display: "GPT 5.6 Luna", alias: "gpt-5.6-luna", providerId: "p1" },
  { id: "m2", upstream: "gpt-5.6-codex", caps: ["工具调用", "128K+上下文"], display: "GPT 5.6 Codex", alias: "codex", providerId: "p1" },
  { id: "m3", upstream: "claude-sonnet-4-5", caps: ["视觉", "深度思考"], display: "Claude Sonnet 4.5", alias: "claude", providerId: "p3" },
  { id: "m4", upstream: "deepseek-v3.2", caps: ["工具调用", "128K+上下文"], display: "DeepSeek V3.2", alias: "deepseek", providerId: "p4" },
  { id: "m5", upstream: "gemini-3-flash", caps: ["视觉", "128K+上下文"], display: "Gemini 3 Flash", alias: "gemini", providerId: "p2" },
];

export const initialDevices: Device[] = [
  { id: "dev_9461c346", name: "我的手机 App", tokenPrefix: "sk-cp-a1b2…f9e3", rpm: null, status: "normal", monthCost: 0 },
  { id: "dev_51ff8a02", name: "笔记本 CLI", tokenPrefix: "sk-cp-77cd…21ab", rpm: 60, status: "normal", monthCost: 0 },
  { id: "dev_c0ffee19", name: "旧平板(停用)", tokenPrefix: "sk-cp-0011…9f0c", rpm: null, status: "disabled", monthCost: 0 },
];

export const initialLogs: LogRow[] = [
  { id: "L-2041", time: "21:02:44", status: 200, model: "gpt-5.6-luna", upstream: "https://zc.gcmod.cn/v1", tokens: 862, latencyMs: 1840, source: "dev_9461c346" },
  { id: "L-2040", time: "20:57:12", status: 200, model: "gpt-5.6-codex", upstream: "https://zc.gcmod.cn/v1", tokens: 1204, latencyMs: 2310, source: "dev_51ff8a02" },
  { id: "L-2039", time: "20:51:03", status: 429, model: "deepseek-v3.2", upstream: "https://oneapi.example.net/v1", tokens: 0, latencyMs: 96, source: "dev_9461c346" },
  { id: "L-2038", time: "20:44:37", status: 200, model: "gpt-5.6-luna", upstream: "https://zc.gcmod.cn/v1", tokens: 428, latencyMs: 1120, source: "dev_51ff8a02" },
  { id: "L-2037", time: "20:31:20", status: 502, model: "claude-sonnet-4-5", upstream: "https://api.anthropic.com/v1", tokens: 0, latencyMs: 8020, source: "dev_9461c346" },
];

export const trend7d = [
  { day: "09-25", req: 0, tok: 0 },
  { day: "09-26", req: 0, tok: 0 },
  { day: "09-27", req: 0, tok: 0 },
  { day: "09-28", req: 3, tok: 940 },
  { day: "09-29", req: 12, tok: 3860 },
  { day: "09-30", req: 9, tok: 2410 },
  { day: "今日", req: 12, tok: 1536 },
];

export const latencyEval = [
  { model: "gpt-5.6-luna", ms: 1840 },
  { model: "gpt-5.6-codex", ms: 2310 },
  { model: "deepseek-v3.2", ms: 640 },
  { model: "gemini-3-flash", ms: 980 },
  { model: "claude-sonnet-4-5", ms: 3120 },
];

export const usage24h: UsageRow[] = [
  { hour: "21:00", provider: "zc.gcmod.cn", model: "gpt-5.6-luna", req: 4, inTok: 812, outTok: 420, cost: 0 },
  { hour: "20:00", provider: "zc.gcmod.cn", model: "gpt-5.6-codex", req: 5, inTok: 1930, outTok: 1204, cost: 0 },
  { hour: "20:00", provider: "oneapi.example.net", model: "deepseek-v3.2", req: 2, inTok: 640, outTok: 0, cost: 0 },
  { hour: "19:00", provider: "api.anthropic.com", model: "claude-sonnet-4-5", req: 1, inTok: 0, outTok: 0, cost: 0 },
];

/* --------------------------- 用量记录 (Usage) --------------------------- */

export type DayUsage = { date: string; tokens: number };

/** 每日消耗走势 · 9月22日 – 10月1日
 *  最后 8 天即截图可见的 9月24日–10月1日；全部 10 天合计 = 884,573,424。 */
export const dailyUsage: DayUsage[] = [
  { date: "9月22日", tokens: 115_000_000 },
  { date: "9月23日", tokens: 115_845_124 },
  { date: "9月24日", tokens: 7_358_100 },
  { date: "9月25日", tokens: 12_480_000 },
  { date: "9月26日", tokens: 9_120_400 },
  { date: "9月27日", tokens: 15_730_000 },
  { date: "9月28日", tokens: 73_581_000 },
  { date: "9月29日", tokens: 94_458_800 },
  { date: "9月30日", tokens: 173_000_000 },
  { date: "10月1日", tokens: 268_000_000 },
];

/** 概览：总 Token 消耗与调用次数（与走势图合计一致：8.85 亿） */
export const usageSummary = {
  totalTokens: 884_573_424,
  calls: 6_531,
  costUsd: 168,
  costCny: 1_126,
};

/** 输入 / 输出 / 缓存 细分 */
export const usageBreakdown = {
  input: { pct: "13.7%", total: 121_000_000, parts: [{ label: "普通输入", value: 121_000_000 }] },
  output: {
    pct: "0.6%",
    total: 5_253_300,
    parts: [
      { label: "内容生成", value: 3_321_500 },
      { label: "思考推理", value: 1_931_700 },
    ],
  },
  cache: { pct: "≈86.2%", total: 758_000_000, parts: [{ label: "缓存读取", value: 758_000_000 }] },
};

export const usageModels = [
  "全部模型", "gpt-5.6-luna", "gpt-5.6-codex", "claude-sonnet-4-5", "deepseek-v3.2", "gemini-3-flash",
];

export const usageSources = ["全部来源", "zc.gcmod.cn", "oneapi.example.net", "api.anthropic.com"];

export const auditRows: AuditRow[] = [
  { time: "2026-10-01 20:41", actor: "admin", action: "更新 Provider 密钥", target: "zc.gcmod.cn", ip: "45.207.198.91", result: "成功" },
  { time: "2026-10-01 20:12", actor: "admin", action: "签发设备 Token", target: "dev_9461c346", ip: "45.207.198.91", result: "成功" },
  { time: "2026-10-01 19:58", actor: "unknown", action: "登录尝试", target: "/admin/login", ip: "103.224.56.7", result: "失败" },
  { time: "2026-09-30 23:02", actor: "admin", action: "回滚配置快照", target: "v12 → v11", ip: "45.207.198.91", result: "成功" },
];

export const snapshots: Snapshot[] = [
  { version: "v13", time: "2026-10-01 20:41", author: "admin", summary: "更新 zc.gcmod.cn 密钥" },
  { version: "v12", time: "2026-09-30 22:40", author: "admin", summary: "新增 gemini 模型别名" },
  { version: "v11", time: "2026-09-29 18:11", author: "admin", summary: "启用多级响应缓存" },
];

export const modelOptions = [
  "gpt-5.6-luna", "codex", "gpt-5.6-codex", "claude", "deepseek", "gemini",
];
