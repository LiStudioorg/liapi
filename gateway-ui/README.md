# AI Gateway · 本地真实后端

前端（Vite + React + shadcn）+ 后端（Express 5 + SQLite）。**无 Mock 数据**，所有页面数据都来自真实数据库读写。

## 快速开始

```bash
npm install
npm run db:init      # 初始化 SQLite + 3 条演示设备
npm run dev          # 同时启动 后端(3002) + 前端(5173)
```

打开 <http://localhost:5173> → 侧边栏「设备 Token」即可增删改查。

也可以分开启动：

```bash
npm run server       # 仅后端 :3002
npm run dev:web      # 仅前端 :5173（已配置 /api 与 /v1 代理）
```

> 重置数据库：`npm run db:reset`

## 数据存储

SQLite 文件：`server/data/gateway.db`（使用 Node 24 内置 `node:sqlite`，无需编译原生模块）。

| 表 | 说明 |
|---|---|
| `devices` | 设备 Token：`id` / `name` / `token` / `rpm` / `status` / `month_cost` |
| `upstreams` | 上游 Provider：`name`(Base URL) / `protocol` / `api_key` / `enabled` |
| `models` | 模型别名映射：`upstream`(真实模型名) / `alias`(客户端别名) / `caps` |
| `logs` | 每次网关调用的用量：`device_id` / `model` / `status` / `in_tokens` / `out_tokens` / `latency_ms` |
| `settings` | 系统设置键值对（预算、缓存、白名单、Webhook…） |
| `audit` | 管理操作审计 |

## 页面与接口对应

所有页面均为真实数据，无 Mock：

| 页面 | 使用的接口 |
|---|---|
| 仪表盘 | `/api/stats/summary`、`/api/stats/daily`、`/api/stats/models`、`/api/logs` |
| Providers | `/api/upstreams`（增删改查）、`/api/upstreams/:id/probe`（真实探测） |
| Models | `/api/models`（增删改查）、`/api/upstreams` |
| Playground | `POST /v1/chat/completions`（真实网关调用） |
| 设备 Token | `/api/tokens`（增删改查 + rotate） |
| 用量记录 | `/api/stats/summary`、`/api/stats/usage`、`/api/stats/daily` |
| 系统设置 | `/api/settings`、`/api/audit`、`/api/settings/cache/clear`、`/api/settings/webhook/test` |


## API

后端默认 `http://127.0.0.1:3002`，已开启 **CORS（允许所有来源 + OPTIONS 预检 204）**，手机可直接访问。

### 设备 Token

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/tokens` | 列表（token 只回传掩码，不回传明文） |
| POST | `/api/tokens` | 新建，自动生成 `dev_xxx` ID 与 `sk-cp-xxx` 密钥；**完整密钥仅在响应中返回一次** |
| PUT | `/api/tokens/:id` | 修改 `name` / `rpm` / `status`（字段可选，只传要改的） |
| DELETE | `/api/tokens/:id` | 删除 |
| POST | `/api/tokens/:id/rotate` | 重新签发密钥 |

`rpm` 传 `null` 表示无限制。

```bash
curl -X POST http://127.0.0.1:3002/api/tokens \
  -H 'Content-Type: application/json' \
  -d '{"name":"Postman 设备","rpm":60}'

curl -X PUT http://127.0.0.1:3002/api/tokens/dev_xxxx \
  -H 'Content-Type: application/json' -d '{"name":"新名字","rpm":300}'

curl -X DELETE http://127.0.0.1:3002/api/tokens/dev_xxxx
```

### 日志与概览

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/logs?n=50` | 最近 n 条调用日志 |
| GET | `/api/overview` | 调用次数 / 总 Token / 设备数 |

### 核心网关代理

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/v1/chat/completions` | 校验 Token → 转发上游 → 记录用量 |
| GET | `/v1/models` | 可用模型列表 |

处理链路：

1. 从 `Authorization: Bearer sk-cp-…`（或 `x-api-key`）取 Token
2. 查库校验：不存在 → `401`；`status=disabled` → `403`
3. RPM 限流：统计最近 60 秒调用次数，超限 → `429`
4. 携带服务端密钥转发到上游 Provider
5. 解析响应 `usage`，把 `prompt_tokens` / `completion_tokens` 与耗时写入 `logs`

```bash
curl -X POST http://127.0.0.1:3002/v1/chat/completions \
  -H 'Authorization: Bearer sk-cp-你的设备密钥' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"你好"}]}'
```

## 上游配置

通过环境变量指定，默认指向 OpenAI 官方：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `3002` | 后端端口 |
| `UPSTREAM_BASE_URL` | `https://api.openai.com/v1` | 上游地址（OpenAI 协议） |
| `UPSTREAM_API_KEY` | 空 | 上游密钥；**未配置时网关返回 500 并记录日志** |
| `UPSTREAM_MODEL` | `gpt-4o-mini` | 默认模型 |

```bash
UPSTREAM_BASE_URL=https://your-provider/v1 \
UPSTREAM_API_KEY=sk-xxx \
npm run server
```

## 目录结构

```
gateway-ui/
├─ server/
│  ├─ db.js        SQLite 初始化 / 建表 / 演示数据
│  ├─ index.js     Express API + 网关代理 + CORS
│  └─ dev.js       一键同启前后端
├─ src/
│  ├─ lib/api.ts       真实 fetch 客户端（无 Mock）
│  ├─ lib/useAsync.ts  加载/错误/重载钩子
│  └─ components/pages/Devices.tsx  增删改查界面
└─ vite.config.ts   /api 与 /v1 开发代理
```
