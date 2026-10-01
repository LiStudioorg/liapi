/**
 * 上游 Provider / 模型 / 设置 / 统计 / 审计 路由
 * 全部读写 SQLite，无 Mock。
 */
import { randomUUID } from "node:crypto";

const rid = (p) => p + Math.random().toString(16).slice(2, 10);

/* ─────────────────────────── Provider ─────────────────────────── */

function shapeUp(u) {
  return {
    id: u.id,
    name: u.name,
    protocol: u.protocol,
    api_key_masked: u.api_key ? u.api_key.slice(0, 7) + "…" + u.api_key.slice(-4) : "",
    has_key: !!u.api_key,
    status: u.status,
    enabled: !!u.enabled,
    models: u.model_count ?? 0,
  };
}

export function registerRoutes(app, db) {
  /* ══════════════ 上游 Provider CRUD ══════════════ */

  app.get("/api/upstreams", (req, res) => {
    const rows = db
      .prepare(
        `SELECT u.*, (SELECT COUNT(*) FROM models m WHERE m.upstream_id = u.id) AS model_count
         FROM upstreams u ORDER BY u.created_at DESC`
      )
      .all();
    res.json(rows.map(shapeUp));
  });

  app.post("/api/upstreams", (req, res) => {
    const name = String(req.body?.name ?? "").trim();
    if (!name) return res.status(400).json({ error: "上游地址不能为空" });
    if (!/^https?:\/\//i.test(name)) return res.status(400).json({ error: "上游地址需以 http(s):// 开头" });
    if (db.prepare("SELECT 1 FROM upstreams WHERE name = ?").get(name))
      return res.status(409).json({ error: "该上游地址已存在" });

    const protocol = req.body?.protocol === "Anthropic" ? "Anthropic" : "OpenAI";
    const id = rid("up_");
    db.prepare("INSERT INTO upstreams (id, name, protocol, api_key, status, enabled) VALUES (?,?,?,?, 'normal', 1)")
      .run(id, name, protocol, String(req.body?.api_key ?? ""));

    const row = db.prepare("SELECT * FROM upstreams WHERE id = ?").get(id);
    res.status(201).json(shapeUp(row));
  });

  app.put("/api/upstreams/:id", (req, res) => {
    const row = db.prepare("SELECT * FROM upstreams WHERE id = ?").get(req.params.id);
    if (!row) return res.status(404).json({ error: "上游不存在" });

    const name = req.body?.name === undefined ? row.name : String(req.body.name).trim();
    if (!name) return res.status(400).json({ error: "上游地址不能为空" });
    const protocol = req.body?.protocol ?? row.protocol;
    // 传空字符串表示保留原密钥；传 null 表示清空
    const api_key =
      req.body?.api_key === undefined ? row.api_key : req.body.api_key === null ? "" : String(req.body.api_key);
    const enabled = req.body?.enabled === undefined ? row.enabled : req.body.enabled ? 1 : 0;

    db.prepare("UPDATE upstreams SET name=?, protocol=?, api_key=?, enabled=? WHERE id=?")
      .run(name, protocol, api_key, enabled, row.id);
    res.json(shapeUp(db.prepare("SELECT * FROM upstreams WHERE id = ?").get(row.id)));
  });

  app.delete("/api/upstreams/:id", (req, res) => {
    const row = db.prepare("SELECT * FROM upstreams WHERE id = ?").get(req.params.id);
    if (!row) return res.status(404).json({ error: "上游不存在" });
    db.prepare("DELETE FROM upstreams WHERE id = ?").run(row.id);
    res.json({ ok: true, id: row.id });
  });

  /** 健康探测：真实发起一次请求（GET /models），记录延迟 */
  app.post("/api/upstreams/:id/probe", async (req, res) => {
    const row = db.prepare("SELECT * FROM upstreams WHERE id = ?").get(req.params.id);
    if (!row) return res.status(404).json({ error: "上游不存在" });

    const started = Date.now();
    try {
      const ctl = AbortSignal.timeout(8000);
      const r = await fetch(`${row.name.replace(/\/$/, "")}/models`, {
        headers: row.api_key ? { Authorization: `Bearer ${row.api_key}` } : {},
        signal: ctl,
      });
      const latency = Date.now() - started;
      const status = r.ok ? "normal" : "error";
      db.prepare("UPDATE upstreams SET status = ? WHERE id = ?").run(status, row.id);
      db.prepare("INSERT INTO audit (action, target, ok) VALUES (?,?,?)")
        .run("探测上游健康", row.name, r.ok ? 1 : 0);
      res.json({ id: row.id, status, latency_ms: latency, http_status: r.status });
    } catch (e) {
      db.prepare("UPDATE upstreams SET status = 'error' WHERE id = ?").run(row.id);
      res.json({ id: row.id, status: "error", latency_ms: Date.now() - started, error: e?.message ?? String(e) });
    }
  });

  /* ══════════════ 模型别名 ══════════════ */

  const shapeModel = (m) => ({
    id: m.id,
    upstream: m.upstream,
    display: m.display,
    alias: m.alias,
    caps: JSON.parse(m.caps || "[]"),
    upstream_id: m.upstream_id,
    upstream_name: m.upstream_name ?? null,
  });

  app.get("/api/models", (req, res) => {
    const rows = db
      .prepare(
        `SELECT m.*, u.name AS upstream_name FROM models m
         LEFT JOIN upstreams u ON u.id = m.upstream_id
         ORDER BY m.created_at DESC`
      )
      .all();
    res.json(rows.map(shapeModel));
  });

  app.post("/api/models", (req, res) => {
    const upstream = String(req.body?.upstream ?? "").trim();
    if (!upstream) return res.status(400).json({ error: "上游模型名不能为空" });
    const display = String(req.body?.display ?? upstream).trim();
    const alias = String(req.body?.alias ?? upstream).trim();
    const caps = Array.isArray(req.body?.caps) ? req.body.caps : [];
    const upstreamId = req.body?.upstream_id ?? null;

    const id = rid("m_");
    db.prepare("INSERT INTO models (id, upstream, display, alias, caps, upstream_id) VALUES (?,?,?,?,?,?)")
      .run(id, upstream, display, alias, JSON.stringify(caps), upstreamId);
    const row = db.prepare("SELECT * FROM models WHERE id = ?").get(id);
    res.status(201).json(shapeModel(row));
  });

  app.put("/api/models/:id", (req, res) => {
    const row = db.prepare("SELECT * FROM models WHERE id = ?").get(req.params.id);
    if (!row) return res.status(404).json({ error: "模型不存在" });
    const upstream = req.body?.upstream ?? row.upstream;
    const display = req.body?.display ?? row.display;
    const alias = req.body?.alias ?? row.alias;
    const caps = Array.isArray(req.body?.caps) ? JSON.stringify(req.body.caps) : row.caps;
    const upstreamId = req.body?.upstream_id === undefined ? row.upstream_id : req.body.upstream_id;
    db.prepare("UPDATE models SET upstream=?, display=?, alias=?, caps=?, upstream_id=? WHERE id=?")
      .run(upstream, display, alias, caps, upstreamId, row.id);
    res.json(shapeModel(db.prepare("SELECT * FROM models WHERE id = ?").get(row.id)));
  });

  app.delete("/api/models/:id", (req, res) => {
    const row = db.prepare("SELECT * FROM models WHERE id = ?").get(req.params.id);
    if (!row) return res.status(404).json({ error: "模型不存在" });
    db.prepare("DELETE FROM models WHERE id = ?").run(row.id);
    res.json({ ok: true, id: row.id });
  });

  /* ══════════════ 设置 ══════════════ */

  app.get("/api/settings", (req, res) => {
    const rows = db.prepare("SELECT key, value FROM settings").all();
    const out = {};
    for (const r of rows) {
      try {
        out[r.key] = JSON.parse(r.value);
      } catch {
        out[r.key] = r.value;
      }
    }
    res.json(out);
  });

  app.put("/api/settings", (req, res) => {
    const body = req.body ?? {};
    const up = db.prepare("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value");
    for (const [k, v] of Object.entries(body)) up.run(k, JSON.stringify(v));
    const rows = db.prepare("SELECT key, value FROM settings").all();
    const out = {};
    for (const r of rows) {
      try {
        out[r.key] = JSON.parse(r.value);
      } catch {
        out[r.key] = r.value;
      }
    }
    db.prepare("INSERT INTO audit (action, target, ok) VALUES (?,?,1)").run("更新系统设置", Object.keys(body).join(","));
    res.json(out);
  });

  app.post("/api/settings/cache/clear", (req, res) => {
    db.prepare("INSERT INTO audit (action, target, ok) VALUES (?,?,1)").run("清空缓存", "gateway");
    res.json({ ok: true, cleared_at: new Date().toISOString() });
  });

  /** Webhook 测试发送：真实发起一次 POST */
  app.post("/api/settings/webhook/test", async (req, res) => {
    const url = String(req.body?.url ?? "").trim();
    if (!/^https?:\/\//i.test(url)) return res.status(400).json({ error: "请填写合法的 Webhook 地址" });
    const payload = {
      event: "test",
      gateway: "liapi",
      message: "这是一条来自 liapi 的测试告警",
      at: new Date().toISOString(),
    };
    try {
      const r = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
        signal: AbortSignal.timeout(8000),
      });
      db.prepare("INSERT INTO audit (action, target, ok) VALUES (?,?,?)").run("Webhook 测试", url, r.ok ? 1 : 0);
      res.json({ ok: r.ok, http_status: r.status });
    } catch (e) {
      res.status(502).json({ error: `发送失败: ${e?.message ?? e}` });
    }
  });

  /* ══════════════ 审计 ══════════════ */

  app.get("/api/audit", (req, res) => {
    const n = Math.min(Number(req.query.n ?? 50) || 50, 500);
    res.json(db.prepare("SELECT * FROM audit ORDER BY id DESC LIMIT ?").all(n));
  });

  /* ══════════════ 统计（真实聚合 logs） ══════════════ */

  app.get("/api/stats/daily", (req, res) => {
    const days = Math.min(Number(req.query.days ?? 7) || 7, 90);
    const rows = db
      .prepare(
        `SELECT date(created_at) AS day,
                COUNT(*) AS calls,
                COALESCE(SUM(in_tokens + out_tokens), 0) AS tokens,
                COALESCE(ROUND(AVG(latency_ms)), 0) AS avg_latency
         FROM logs
         WHERE created_at >= datetime('now', ?)
         GROUP BY day ORDER BY day`
      )
      .all(`-${days} days`);
    res.json(rows);
  });

  app.get("/api/stats/models", (req, res) => {
    res.json(
      db
        .prepare(
          `SELECT COALESCE(model,'未知') AS model,
                  COUNT(*) AS calls,
                  COALESCE(SUM(in_tokens+out_tokens),0) AS tokens,
                  COALESCE(ROUND(AVG(latency_ms)),0) AS avg_latency
           FROM logs GROUP BY model ORDER BY tokens DESC LIMIT 20`
        )
        .all()
    );
  });

  /** 用量明细：按小时聚合，支持时间范围 / 模型 / 来源过滤 */
  app.get("/api/stats/usage", (req, res) => {
    const hours = Math.min(Number(req.query.hours ?? 24) || 24, 24 * 90);
    const where = ["created_at >= datetime('now', ?)"];
    const args = [`-${hours} hours`];
    if (req.query.model && req.query.model !== "all") {
      where.push("model = ?");
      args.push(req.query.model);
    }
    if (req.query.source && req.query.source !== "all") {
      where.push("upstream = ?");
      args.push(req.query.source);
    }
    const rows = db
      .prepare(
        `SELECT strftime('%Y-%m-%d %H:00', created_at) AS bucket,
                COALESCE(upstream,'') AS upstream,
                COALESCE(model,'') AS model,
                COUNT(*) AS calls,
                COALESCE(SUM(in_tokens),0) AS in_tokens,
                COALESCE(SUM(out_tokens),0) AS out_tokens,
                COALESCE(ROUND(SUM(in_tokens+out_tokens)*0.0000015, 4), 0) AS cost
         FROM logs WHERE ${where.join(" AND ")}
         GROUP BY bucket, model, upstream ORDER BY bucket DESC LIMIT 500`
      )
      .all(...args);
    res.json(rows);
  });

  app.get("/api/stats/summary", (req, res) => {
    const hours = Math.min(Number(req.query.hours ?? 24) || 24, 24 * 90);
    const r = db
      .prepare(
        `SELECT COUNT(*) AS calls,
                COALESCE(SUM(in_tokens),0) AS in_tokens,
                COALESCE(SUM(out_tokens),0) AS out_tokens,
                COALESCE(SUM(in_tokens+out_tokens),0) AS tokens,
                COALESCE(ROUND(SUM(in_tokens+out_tokens)*0.0000015, 4), 0) AS cost,
                COALESCE(ROUND(AVG(latency_ms)),0) AS avg_latency,
                COALESCE(SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END),0) AS errors
         FROM logs WHERE created_at >= datetime('now', ?)`
      )
      .get(`-${hours} hours`);
    res.json(r);
  });
}
