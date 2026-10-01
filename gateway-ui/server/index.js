/**
 * liapi 本地后端
 *
 * 启动：npm run server      （或 npm run dev 同时起前端+后端）
 * 端口：3002（可用 PORT 覆盖）
 *
 * 接口：
 *   GET    /api/tokens            设备 Token 列表
 *   POST   /api/tokens            新建（自动生成 dev_xxx）
 *   PUT    /api/tokens/:id        修改名称 / RPM / 状态
 *   DELETE /api/tokens/:id        删除
 *   POST   /api/tokens/:id/rotate 重新签发密钥
 *   GET    /api/logs              调用日志
 *   GET    /api/overview          汇总统计
 *   POST   /v1/chat/completions   核心网关代理（校验 Token → 转发上游 → 记录用量）
 *   GET    /v1/models             模型列表
 */
import express from "express";
import { initDb, genDeviceId, genToken } from "./db.js";
import { registerRoutes } from "./routes.js";

const PORT = Number(process.env.PORT ?? 3002);

/** 上游 Provider 配置：可用环境变量覆盖，默认指向 OpenAI 官方 */
const UPSTREAM_BASE = process.env.UPSTREAM_BASE_URL ?? "https://api.openai.com/v1";
const UPSTREAM_KEY = process.env.UPSTREAM_API_KEY ?? "";
const UPSTREAM_MODEL = process.env.UPSTREAM_MODEL ?? "gpt-4o-mini";

const db = initDb();

const app = express();
app.use(express.json({ limit: "10mb" }));

/* ───────────────────────────  CORS  ───────────────────────────
 * 允许所有来源（含手机浏览器）；预检请求直接 204 放行。 */
app.use((req, res, next) => {
  res.setHeader("Access-Control-Allow-Origin", "*");
  res.setHeader("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS");
  res.setHeader("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key");
  res.setHeader("Access-Control-Max-Age", "86400");
  if (req.method === "OPTIONS") return res.sendStatus(204);
  next();
});

// 上游 / 模型 / 设置 / 统计 / 审计 路由
registerRoutes(app, db);

/** 统一把设备行转成前端要的形状（不回传完整 token，只给掩码） */
function shape(d) {
  return {
    id: d.id,
    name: d.name,
    token_masked: d.token.slice(0, 11) + "…" + d.token.slice(-4),
    rpm: d.rpm,
    status: d.status,
    month_cost: d.month_cost,
    created_at: d.created_at,
    updated_at: d.updated_at,
  };
}

const clampRpm = (v) => {
  if (v === null || v === undefined || v === "" || v === "none") return null;
  const n = Number(v);
  if (!Number.isFinite(n) || n <= 0) return null;
  return Math.min(Math.round(n), 100000);
};

/* ══════════════════════ 设备 Token CRUD ══════════════════════ */

// GET /api/tokens
app.get("/api/tokens", (req, res) => {
  const rows = db.prepare("SELECT * FROM devices ORDER BY created_at DESC, rowid DESC").all();
  res.json(rows.map(shape));
});

// POST /api/tokens
app.post("/api/tokens", (req, res) => {
  const name = String(req.body?.name ?? "").trim();
  if (!name) return res.status(400).json({ error: "设备名称不能为空" });

  const id = genDeviceId();
  const token = genToken();
  const rpm = clampRpm(req.body?.rpm);

  db.prepare(
    "INSERT INTO devices (id, name, token, rpm, status, month_cost) VALUES (?,?,?,?, 'normal', 0)"
  ).run(id, name, token, rpm);

  const row = db.prepare("SELECT * FROM devices WHERE id = ?").get(id);
  // 完整 token 只在创建时返回一次。
  res.status(201).json({ ...shape(row), token });
});

// PUT /api/tokens/:id
app.put("/api/tokens/:id", (req, res) => {
  const row = db.prepare("SELECT * FROM devices WHERE id = ?").get(req.params.id);
  if (!row) return res.status(404).json({ error: "设备不存在" });

  const name = req.body?.name === undefined ? row.name : String(req.body.name).trim();
  if (!name) return res.status(400).json({ error: "设备名称不能为空" });
  const rpm = req.body?.rpm === undefined ? row.rpm : clampRpm(req.body.rpm);
  const status =
    req.body?.status === undefined
      ? row.status
      : req.body.status === "disabled"
      ? "disabled"
      : "normal";

  db.prepare(
    "UPDATE devices SET name = ?, rpm = ?, status = ?, updated_at = datetime('now') WHERE id = ?"
  ).run(name, rpm, status, row.id);

  res.json(shape(db.prepare("SELECT * FROM devices WHERE id = ?").get(row.id)));
});

// DELETE /api/tokens/:id
app.delete("/api/tokens/:id", (req, res) => {
  const row = db.prepare("SELECT * FROM devices WHERE id = ?").get(req.params.id);
  if (!row) return res.status(404).json({ error: "设备不存在" });
  db.prepare("DELETE FROM devices WHERE id = ?").run(row.id);
  res.json({ ok: true, id: row.id });
});

// POST /api/tokens/:id/rotate —— 重新签发密钥
app.post("/api/tokens/:id/rotate", (req, res) => {
  const row = db.prepare("SELECT * FROM devices WHERE id = ?").get(req.params.id);
  if (!row) return res.status(404).json({ error: "设备不存在" });
  const token = genToken();
  db.prepare("UPDATE devices SET token = ?, updated_at = datetime('now') WHERE id = ?").run(token, row.id);
  res.json({ ...shape(db.prepare("SELECT * FROM devices WHERE id = ?").get(row.id)), token });
});

/* ══════════════════════ 日志与概览 ══════════════════════ */

app.get("/api/logs", (req, res) => {
  const n = Math.min(Number(req.query.n ?? 50) || 50, 500);
  res.json(db.prepare("SELECT * FROM logs ORDER BY id DESC LIMIT ?").all(n));
});

app.get("/api/overview", (req, res) => {
  const calls = db.prepare("SELECT COUNT(*) c FROM logs").get().c;
  const tok = db.prepare("SELECT COALESCE(SUM(in_tokens+out_tokens),0) t FROM logs").get().t;
  const devices = db.prepare("SELECT COUNT(*) c FROM devices").get().c;
  res.json({ calls, tokens: tok, devices });
});

/* ══════════════════════ 核心网关代理 ══════════════════════ */

/** 从 Authorization: Bearer xxx 或 x-api-key 取 Token */
function clientToken(req) {
  const auth = req.header("authorization") ?? "";
  if (auth.toLowerCase().startsWith("bearer ")) return auth.slice(7).trim();
  return (req.header("x-api-key") ?? "").trim();
}

// GET /v1/models
app.get("/v1/models", (req, res) => {
  res.json({
    object: "list",
    data: [
      { id: UPSTREAM_MODEL, object: "model", owned_by: "gateway" },
      { id: "gpt-5.6-luna", object: "model", owned_by: "gateway" },
      { id: "deepseek-v3.2", object: "model", owned_by: "gateway" },
    ],
  });
});

// POST /v1/chat/completions
app.post("/v1/chat/completions", async (req, res) => {
  const started = Date.now();
  const token = clientToken(req);
  if (!token) {
    return res.status(401).json({ error: { message: "缺少 API Key", type: "invalid_request_error" } });
  }

  const dev = db.prepare("SELECT * FROM devices WHERE token = ?").get(token);
  if (!dev) {
    return res.status(401).json({ error: { message: "无效的 API Key", type: "invalid_request_error" } });
  }
  if (dev.status !== "normal") {
    return res.status(403).json({ error: { message: "该设备已被禁用", type: "permission_error" } });
  }

  // RPM 限流：统计最近 60 秒内该设备的调用次数
  if (dev.rpm) {
    const used = db
      .prepare("SELECT COUNT(*) c FROM logs WHERE device_id = ? AND created_at > datetime('now','-60 seconds')")
      .get(dev.id).c;
    if (used >= dev.rpm) {
      return res.status(429).json({ error: { message: `超出每分钟限流 ${dev.rpm} 次`, type: "rate_limit_error" } });
    }
  }

  const model = req.body?.model ?? UPSTREAM_MODEL;

  // 从数据库解析上游：优先使用启用的 Provider，支持模型别名改写
  const upstreamRow =
    db.prepare("SELECT * FROM upstreams WHERE enabled = 1 AND status = 'normal' ORDER BY created_at LIMIT 1").get() ??
    db.prepare("SELECT * FROM upstreams WHERE enabled = 1 ORDER BY created_at LIMIT 1").get();

  const base = upstreamRow?.name ?? UPSTREAM_BASE;
  const key = upstreamRow?.api_key || UPSTREAM_KEY;

  // 别名 → 上游真实模型名
  const aliasRow = db.prepare("SELECT upstream FROM models WHERE alias = ?").get(model);
  const realModel = aliasRow?.upstream ?? model;

  const logErr = (status, msg) => {
    db.prepare(
      "INSERT INTO logs (device_id, model, upstream, status, in_tokens, out_tokens, latency_ms, error) VALUES (?,?,?,?,0,0,?,?)"
    ).run(dev.id, model, base, status, Date.now() - started, msg);
  };

  if (!key) {
    const msg = "未配置上游密钥：请在 Providers 页面添加并填写 API Key";
    logErr(500, msg);
    return res.status(500).json({ error: { message: msg, type: "configuration_error" } });
  }

  try {
    const upstream = await fetch(`${base.replace(/\/$/, "")}/chat/completions`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${key}` },
      body: JSON.stringify({ ...req.body, model: realModel }),
    });

    const text = await upstream.text();

    if (!upstream.ok) {
      logErr(upstream.status, text.slice(0, 500));
      res.status(upstream.status);
      res.setHeader("Content-Type", upstream.headers.get("content-type") ?? "application/json");
      return res.send(text);
    }

    // 记录真实用量
    let inTok = 0;
    let outTok = 0;
    try {
      const j = JSON.parse(text);
      inTok = j?.usage?.prompt_tokens ?? 0;
      outTok = j?.usage?.completion_tokens ?? 0;
    } catch {
      /* 流式或非 JSON：用量记 0 */
    }
    db.prepare(
      "INSERT INTO logs (device_id, model, upstream, status, in_tokens, out_tokens, latency_ms) VALUES (?,?,?,?,?,?,?)"
    ).run(dev.id, model, base, upstream.status, inTok, outTok, Date.now() - started);

    res.setHeader("Content-Type", upstream.headers.get("content-type") ?? "application/json");
    res.send(text);
  } catch (e) {
    const msg = `上游请求失败: ${e?.message ?? e}`;
    logErr(502, msg);
    res.status(502).json({ error: { message: msg, type: "upstream_error" } });
  }
});

app.use((req, res) => res.status(404).json({ error: "Not Found: " + req.path }));

app.listen(PORT, "0.0.0.0", () => {
  console.log(`\n  liapi 后端已启动`);
  console.log(`  ├─ 本机     http://127.0.0.1:${PORT}`);
  console.log(`  ├─ 局域网   http://0.0.0.0:${PORT}`);
  console.log(`  ├─ 设备接口 http://127.0.0.1:${PORT}/api/tokens`);
  console.log(`  └─ 网关代理 POST /v1/chat/completions`);
  console.log(`  上游: ${UPSTREAM_BASE}  ${UPSTREAM_KEY ? "(已配置密钥)" : "(⚠ 未配置 UPSTREAM_API_KEY)"}\n`);
});
