/**
 * SQLite 初始化脚本
 *
 * 使用 Node 24 内置的 node:sqlite（无需编译原生模块，npm install 不会失败）。
 * 运行：npm run db:init      —— 建表 + 写入演示数据
 *      npm run db:reset     —— 删库重建
 *
 * 表结构：
 *   devices  设备 Token（本页面的 CRUD 对象）
 *   logs     每次 /v1/chat/completions 调用的用量日志
 */
import { DatabaseSync } from "node:sqlite";
import { mkdirSync, rmSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
export const DB_PATH = process.env.DB_PATH ?? resolve(__dirname, "data/gateway.db");

/** 生成 sk-cp-xxxx 形式的密钥 */
export function genToken() {
  const hex = () => Math.random().toString(16).slice(2, 10);
  return `sk-cp-${hex()}${hex()}${hex()}`;
}

/** 生成 dev_xxxxxxxx 形式的设备 ID */
export function genDeviceId() {
  return "dev_" + Math.random().toString(16).slice(2, 10);
}

export function openDb() {
  mkdirSync(dirname(DB_PATH), { recursive: true });
  const db = new DatabaseSync(DB_PATH);
  db.exec("PRAGMA journal_mode = WAL");
  db.exec("PRAGMA foreign_keys = ON");
  return db;
}

export function migrate(db) {
  db.exec(`
    CREATE TABLE IF NOT EXISTS devices (
      id          TEXT PRIMARY KEY,
      name        TEXT NOT NULL,
      token       TEXT NOT NULL UNIQUE,
      rpm         INTEGER,                      -- NULL = 无限制
      status      TEXT NOT NULL DEFAULT 'normal' CHECK (status IN ('normal','disabled')),
      month_cost  REAL NOT NULL DEFAULT 0,
      created_at  TEXT NOT NULL DEFAULT (datetime('now')),
      updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS logs (
      id           INTEGER PRIMARY KEY AUTOINCREMENT,
      device_id    TEXT,
      model        TEXT,
      upstream     TEXT,
      status       INTEGER,
      in_tokens    INTEGER NOT NULL DEFAULT 0,
      out_tokens   INTEGER NOT NULL DEFAULT 0,
      latency_ms   INTEGER NOT NULL DEFAULT 0,
      error        TEXT,
      created_at   TEXT NOT NULL DEFAULT (datetime('now'))
    );

    CREATE INDEX IF NOT EXISTS idx_logs_created ON logs(created_at DESC);
    CREATE INDEX IF NOT EXISTS idx_logs_device  ON logs(device_id);

    -- 上游 Provider（OpenAI / Anthropic 协议）
    CREATE TABLE IF NOT EXISTS upstreams (
      id          TEXT PRIMARY KEY,
      name        TEXT NOT NULL UNIQUE,          -- Base URL，如 https://api.openai.com/v1
      protocol    TEXT NOT NULL DEFAULT 'OpenAI' CHECK (protocol IN ('OpenAI','Anthropic')),
      api_key     TEXT NOT NULL DEFAULT '',
      status      TEXT NOT NULL DEFAULT 'normal' CHECK (status IN ('normal','error')),
      enabled     INTEGER NOT NULL DEFAULT 1,
      created_at  TEXT NOT NULL DEFAULT (datetime('now'))
    );

    -- 模型别名映射
    CREATE TABLE IF NOT EXISTS models (
      id           TEXT PRIMARY KEY,
      upstream     TEXT NOT NULL,                -- 上游真实模型名
      display      TEXT NOT NULL,                -- 显示名称
      alias        TEXT NOT NULL,                -- 客户端别名
      caps         TEXT NOT NULL DEFAULT '[]',   -- JSON 数组：视觉/工具调用/…
      upstream_id  TEXT REFERENCES upstreams(id) ON DELETE SET NULL,
      created_at   TEXT NOT NULL DEFAULT (datetime('now'))
    );

    -- 系统设置：键值对
    CREATE TABLE IF NOT EXISTS settings (
      key   TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );

    -- 管理员操作审计
    CREATE TABLE IF NOT EXISTS audit (
      id         INTEGER PRIMARY KEY AUTOINCREMENT,
      actor      TEXT NOT NULL DEFAULT 'admin',
      action     TEXT NOT NULL,
      target     TEXT,
      ip         TEXT,
      ok         INTEGER NOT NULL DEFAULT 1,
      created_at TEXT NOT NULL DEFAULT (datetime('now'))
    );
  `);
}

function seed(db) {
  const n = db.prepare("SELECT COUNT(*) AS c FROM devices").get().c;
  if (n === 0) {
    const ins = db.prepare(
      "INSERT INTO devices (id, name, token, rpm, status, month_cost) VALUES (?,?,?,?,?,?)"
    );
    ins.run(genDeviceId(), "我的手机 App", genToken(), null, "normal", 0);
    ins.run(genDeviceId(), "笔记本 CLI", genToken(), 60, "normal", 0);
    ins.run(genDeviceId(), "旧平板(停用)", genToken(), null, "disabled", 0);
  }

  // 上游 Provider：默认用环境变量里的配置，方便开箱即用
  if (db.prepare("SELECT COUNT(*) AS c FROM upstreams").get().c === 0) {
    const ins = db.prepare(
      "INSERT INTO upstreams (id, name, protocol, api_key, status, enabled) VALUES (?,?,?,?,?,?)"
    );
    const base = process.env.UPSTREAM_BASE_URL ?? "https://api.openai.com/v1";
    const key = process.env.UPSTREAM_API_KEY ?? "";
    ins.run("up_" + Math.random().toString(16).slice(2, 10), base, "OpenAI", key, "normal", 1);
  }

  // 模型别名
  if (db.prepare("SELECT COUNT(*) AS c FROM models").get().c === 0) {
    const up = db.prepare("SELECT id FROM upstreams LIMIT 1").get();
    const ins = db.prepare(
      "INSERT INTO models (id, upstream, display, alias, caps, upstream_id) VALUES (?,?,?,?,?,?)"
    );
    ins.run("m1", "gpt-4o-mini", "GPT-4o mini", "gpt-5.6-luna", JSON.stringify(["视觉", "工具调用", "128K+上下文"]), up?.id ?? null);
    ins.run("m2", "gpt-4o", "GPT-4o", "gpt-5.6-codex", JSON.stringify(["视觉", "工具调用", "128K+上下文"]), up?.id ?? null);
    ins.run("m3", "deepseek-chat", "DeepSeek Chat", "deepseek-v3.2", JSON.stringify(["工具调用", "深度思考"]), up?.id ?? null);
  }

  // 默认设置
  const defaults = {
    site_name: "liapi",
    ip_whitelist_enabled: "false",
    ip_whitelist: JSON.stringify(["127.0.0.1"]),
    cache_enabled: "true",
    cache_ttl: "300",
    monthly_budget: "20",
    budget_action: "notify",
    budget_threshold: "80",
    webhook_url: "",
    login_enabled: "false",
    admin_username: "admin",
  };
  const ins = db.prepare("INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)");
  for (const [k, v] of Object.entries(defaults)) ins.run(k, v);
}

/** 供服务端直接调用：确保库存在且已建表 */
export function initDb({ reset = false } = {}) {
  if (reset) {
    // 必须把 WAL/shm 一并删掉，否则残留的写日志会和新库冲突，
    // 导致进程句柄与磁盘文件脱节（表现为数据"反复消失"）。
    for (const suffix of ["", "-wal", "-shm"]) {
      rmSync(DB_PATH + suffix, { force: true });
    }
  }
  const db = openDb();
  migrate(db);
  seed(db);
  return db;
}

// 作为脚本直接运行时：初始化并打印结果
if (import.meta.url === `file://${process.argv[1]}`) {
  const reset = process.argv.includes("--reset");
  const db = initDb({ reset });
  const rows = db.prepare("SELECT id, name, rpm, status FROM devices ORDER BY created_at").all();
  console.log(`✅ 数据库就绪：${DB_PATH}`);
  console.log(`   设备数：${rows.length}`);
  for (const r of rows) console.log(`   - ${r.id}  ${r.name}  rpm=${r.rpm ?? "无限制"}  ${r.status}`);
  db.close();
}
