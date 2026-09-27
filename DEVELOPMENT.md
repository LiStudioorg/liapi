# Liapi 开发文档

> 面向贡献者与维护者的开发指南。功能规格与配置字段见 [README.md](README.md)；
> 协作规范见 [CONTRIBUTING.md](CONTRIBUTING.md)；安全策略见 [SECURITY.md](SECURITY.md)。

---

## 0. 铁律（Must Follow）

> **任何上传至仓库的内容，必须推送到 `https://github.com/LiStudioorg/liapi` 仓库的 `beta` 分支，严禁推送到 `main` 分支。**

- 所有 commit / push 一律以 `beta` 为目标分支，`main` 分支禁止直接写入。
- 当前工作分支即 `beta`（`git branch --show-current` 应为 `beta`）。
- 提交前务必确认目标分支：

  ```bash
  git branch --show-current        # 期望输出：beta
  git push origin beta             # 只推 beta，禁止 git push origin main
  ```

- 合并到 `main`（如需要）只能通过受控的 Pull Request 由维护者执行，开发者本人不得直接推送。

---

## 1. 项目定位

Liapi 是自托管、OpenAI 兼容的 API 网关：对外暴露 `/v1/*`，对内聚合多个上游
（OpenAI / Claude / OpenRouter / 硅基流动 / Ollama…），统一 **鉴权、路由、日志、
限流、配额、故障转移、可观测**。

| 是 | 不是 |
|---|---|
| 自托管 HTTP 服务 | 多租户 SaaS |
| OpenAI 兼容网关 | 不做注册 / 支付 / 发票 |
| 统一鉴权 + 路由 + 限流 + 日志 | 不做模型推理 |

- **技术栈：Go 1.22+（约定 `go 1.22`）/ 纯标准库，零第三方依赖。**
- `go.mod` 必须保持 stdlib-only，这是硬约束（见 CONTRIBUTING）。
- 单个二进制 + 内嵌管理台（`//go:embed`），`go build` 一步出产物。

---

## 2. 架构总览

```
                        ┌──────────────────────────────────────┐
                        │                liapi                 │
   客户端 ────────────►  │  /v1/*   业务入口（OpenAI 兼容）        │
   (SDK / base_url)     │   ├─ 客户端鉴权（token / device / 策略） │
                        │   ├─ 限流 + 日配额                      │
                        │   ├─ 别名解析 / 参数覆写                 │
                        │   ├─ 路由（fallback / priority /        │
                        │   │        latency / cost / group）     │
                        │   ├─ 转发 + 重试 + 故障转移              │
                        │   │    ├─ 流式(SSE) / 非流式             │
                        │   │    └─ 用量抽取                       │
                        │   └─ 日志（JSONL + 环形缓冲）            │
                        │                                      │
                        │  /admin/api/*  管理接口（独立鉴权）       │
                        │  /             内嵌管理台（单文件）       │
                        │  /metrics      Prometheus 文本          │
                        │                                      │
                        │  后台 goroutine：健康探测 / 配额清理 /    │
                        │                 告警 / 日志保留           │
                        └──────────────────────────────────────┘
                             │         │          │
                        ┌────┴───┐ ┌───┴─────┐ ┌──┴──────┐
                        │ OpenAI │ │OpenRouter│ │ Ollama  │ ...
                        └────────┘ └─────────┘ └─────────┘
```

**关键设计（改动时务必保持）：**

1. 上游 `base_url` **自带版本前缀**（如 `https://api.openai.com/v1`）；转发路径 =
   `base_url`（去尾斜杠）+ 客户端路径去掉 `/v1` 前缀（`relativePath`）。
2. 客户端 token 与上游 API Key **完全分离**：前者校验入站，后者只注入出站。
3. 故障转移决策必须在**首次 `WriteHeader` 之前**完成；一旦写响应头不再切换上游。
4. 流式转发**不设 `http.Client.Timeout`**；非流式用 `context.WithTimeout`，
   流式用空闲看门狗（`stream_idle_timeout`），靠客户端断开驱动结束。

---

## 3. 目录结构

```
liapi/
├── go.mod                    # module liapi / go 1.22（零依赖）
├── main.go                   # 入口：加载配置、装配依赖、启动 HTTP、优雅退出
├── config/
│   ├── config.go             # Config/Upstream/Device/AliasRule/... + 默认值 + Validate + 原子 Save
│   ├── holder.go             # 配置 Holder（RWMutex，热重载单一数据源）
│   └── interop.go            # OneAPI channels 导入
├── common/
│   ├── errors.go             # OpenAI 错误格式 {"error":{...}} 与写出
│   └── token.go              # 常量时间比较 / SHA-256 / 脱敏
├── auth/
│   ├── auth.go               # 客户端 token / 设备 token 校验
│   └── admin.go              # 管理 token 校验 + 每 IP 失败锁定
├── routing/
│   └── router.go             # 别名解析 + 候选收集 + fallback/strategy/group/健康过滤
├── relay/
│   ├── relay.go              # 转发主循环（重试 / 故障转移 / 失败分类）
│   ├── stream.go             # SSE 流式复制（Flusher）
│   ├── usage.go              # 用量抽取（OpenAI / Anthropic 两套字段）
│   └── http.go               # 头透传、响应头清洗、尾缓冲
├── server/
│   ├── server.go             # ServeMux 路由注册 + admin 中间件（IP/锁定/限流/token）
│   ├── handler_v1.go         # /v1/* 业务实现
│   ├── handler_admin.go      # /admin/api/* 实现 + SPA 静态托管
│   ├── requestid.go          # X-Request-ID 中间件
│   └── adminui/              # 管理台构建产物（go:embed all:adminui），勿手改
├── web/                      # 管理台源码（Nuxt 4 + fuxsto-design，独立于 Go）
│   ├── nuxt.config.ts        # ssr:false + nitro.preset=static，产物输出到 ../server/adminui
│   ├── package.json          # Nuxt 4 / Vue 3 / Tailwind v4 / fuxsto-design
│   └── app/
│       ├── app.vue           # 根组件
│       ├── layouts/default.vue  # NewAPI 风格侧边栏 + 顶栏 + 路由过渡
│       ├── pages/*.vue       # 概览/上游/令牌/设备/统计/日志/健康/配置/调试
│       ├── components/       # Field / DialogPanel / StatTile
│       ├── composables/      # useApi（admin token + fetch）/ useUi（toast/confirm）
│       ├── types/api.ts      # 与 Go 端 JSON 契约对应的类型
│       └── assets/css/main.css
├── stats/
│   ├── logger.go             # JSONL 写入 + 环形缓冲 + 保留策略
│   ├── limiter.go            # 滑动窗口限流
│   ├── quota.go              # 每日配额 + 告警阈值
│   ├── health.go             # 后台健康探测 + 持久化
│   ├── totals.go             # 原子聚合计数
│   ├── aggregate.go          # 统计聚合（分维度 / 分位数）
│   ├── metrics.go            # Prometheus 指标
│   ├── alerts.go             # Webhook / Bark 告警 + 去重
│   └── audit.go              # 管理端访问审计环
└── .github/workflows/{ci,release}.yml
```

---

## 4. 核心数据流

### 4.1 请求链路（以 `POST /v1/chat/completions` 为例）

```
RequestID 中间件（生成/回显 X-Request-ID）
  ├─ 客户端鉴权：Authorization: Bearer → x-api-key → ?token=
  │    常量时间比较；设备 token 仅比对 SHA-256；策略校验（过期 / IP 白名单）
  ├─ 限流：滑动窗口（全局 rate_limit_per_minute / 设备 rpm / 策略 rpm）→ 429
  ├─ 日配额：daily_per_token / 设备 daily → 超额 429 + 告警
  ├─ 读 body（MaxBytesReader）→ 413；非法 JSON → 400
  ├─ 解析 model / stream；缺 model → 400
  ├─ 别名解析（aliases / alias_rules）+ 参数覆写 → 最终 model
  ├─ 路由：group 过滤 → fallbacks 链 或 strategy 排序 → 健康过滤
  ├─ 转发主循环（重试 + 故障转移）
  ├─ 回写：SSE 流式 / 普通复制（清洗响应头）
  ├─ 用量抽取（usage）→ totals / aggregate
  └─ 写日志（JSONL + 环形缓冲）
```

### 4.2 路由（`routing/router.go`）

1. **别名**：`aliases[model]` 精确优先；否则首条匹配的 `alias_rules`（精确或正则），
   并带回 `params` 覆写。
2. **候选**：遍历启用上游，按 `group` 与 `Matches(model)`（支持 `"*"` 兜底）过滤。
3. **排序**：
   - 若 `fallbacks[model]` 存在 → 按显式链顺序（仅保留匹配到的上游）。
   - 否则按 `priority` 升序分组，组内按 `route_strategy`：
     - `""`/`priority`：`weight` 加权随机（展开 → 洗牌 → 去重保序）。
     - `latency`：`weight * (1000 / max(latency_ms, 50))`，clamp 到 `[1,1000]` 后加权随机。
     - `cost`：按 `prices[model].Input` 升序；无价目则退化为 `priority`。
4. **健康过滤**：`skip_unhealthy=true` 时剔除不健康上游；若全被剔除则**回退全量**
   （防止全挂不可用）。

### 4.3 转发与故障转移（`relay/relay.go`）

失败分类 `failKind`：`failNetwork`（连接失败，安全重试）、`failTimeout`
（可能已入账）、`failUpstream`（429/5xx）。

| 情况 | 处理 |
|---|---|
| `2xx` | 回写并向客户端返回；结束 |
| `4xx`（≠429） | **不切换**，原样透传上游错误 |
| `429` / `5xx` | 同上游重试（受 `upstream.retry` 限制），再切换下一候选 |
| 网络错误 | 同上游重试（安全），再切换 |
| 超时 | 仅当 `retry_on_timeout=true` 才重试/切换（防重复计费） |
| 客户端断开 | 记 `499`，立即停止，**绝不重试** |

- 单上游重试有 `250ms * attempt` 的退避。
- 每次重试/切换通过 `OnEvent` 上报指标与告警。

### 4.4 流式（`relay/stream.go`）

- 以响应 `Content-Type` 含 `text/event-stream` 判断流式。
- 逐行读 + 每行 `http.Flusher.Flush()`；客户端 `Write` 出错即停止并关闭上游 body。
- 响应头清洗：**删除 `Content-Length / Transfer-Encoding / Connection`**，其余透传。
- 空闲看门狗按 `stream_idle_timeout` 断开长时间无数据连接。

---

## 5. 配置与热重载

- 全部配置由管理台（`http://<host>:8787/`）编辑，保存即热重载。
- `config.json` 只作首次引导（自动生成、含随机 `admin_token`），权限强制 `0600`；
  Windows 跳过权限校验，可用 `LIAPI_SKIP_PERM_CHECK=1` 临时绕过。
- 保存流程（`config.Save`）：临时文件 → `fsync` → `os.Rename` 原子替换 → 再次 `chmod 0600`。
- 热重载：`Holder.Set(newCfg)`；所有请求**每次现取** `holder.Get()`，不缓存。
- 管理台是 Nuxt 生成的静态 SPA，产物位于 `server/adminui/`，由
  `server/server.go` 的 `//go:embed all:adminui` 打包进二进制；
  `handler_admin.go` 的 `adminUI` 负责静态资源 + SPA 回退（未知路径 → `index.html`）。
- 构建产物**已提交**（CI/Docker 可在无 Node 环境下 `go build`）；
  改动 `web/` 后必须运行 `./buildadmin.sh` 并提交 `server/adminui/`。

---

## 6. 安全约定（不可违背）

1. **永不记录密钥**：token 用 `common.MaskToken` 脱敏；`api_key` 一律不入日志。
   设备 token 只存 SHA-256（`token_hash`），明文仅在创建/轮换时返回一次。
2. **常量时间比较**：token 校验用 SHA-256 摘要 + `subtle.ConstantTimeCompare`。
3. **管理面三层防护**：IP 白名单（`admin_allow_ips`）→ 每 IP 失败锁定 → 每 IP 限流
   （`admin_rate_per_minute`）→ admin token。每次尝试写入审计环。
4. **X-Forwarded-For 仅在配置白名单时信任**（防伪造）。
5. 日志文件 `0600`；配置文件 `0600`。

---

## 7. 开发环境与常用命令

```bash
git clone https://github.com/LiStudioorg/liapi.git
cd liapi
git checkout beta              # 铁律：工作在 beta 分支
go test ./...
go build -o liapi .
./liapi -config config.json    # 首次运行生成 config.json（0600），打印 admin_token
```

冒烟：

```bash
curl http://localhost:8787/v1/models -H "Authorization: Bearer sk-client-aaa"
curl http://localhost:8787/v1/chat/completions -H "Authorization: Bearer sk-client-aaa" \
     -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}' -N
```

全新装配依赖关系（`main.go`）：

```
config.Load → Holder → stats.NewLogger / NewLimiter / NewTotals
  → relay.New(holder) → stats.NewHealth(holder, relay.Client())
  → routing.NewRouter(holder, health) → auth.NewClient / NewAdmin(holder)
  → server.New(...) → health.Start(ctx) → http.Server
```

---

## 7.1 管理台前端（Nuxt 4 + fuxsto-design）

管理台源码在 `web/`，与 Go 代码完全解耦；**唯一契约是 `/admin/api/*` 的 JSON**。

- 技术栈：Nuxt 4（`ssr: false`）+ Vue 3 + Tailwind CSS v4 + `fuxsto-design` 组件库。
- 构建：`nuxt generate` + `nitro.preset=static`，`output.publicDir` 指向
  `../server/adminui`，产出纯静态 SPA（客户端路由）。
- 布局参考 NewApi：左侧固定侧边栏 + 顶栏，页面切换带过渡动画
  （`pageTransition`，`mode: out-in`）。
- API 客户端：`app/composables/useApi.ts`——admin token 存 `localStorage`，
  请求带 `Authorization: Bearer <admin_token>`，401 时提示重新输入。
- 类型契约：`app/types/api.ts`，字段须与 `server/handler_admin.go` 保持一致。

常用命令（在 `web/` 内）：

```bash
npm install          # 安装依赖（首次）
npm run dev          # 开发服务器（默认 3000，需自行代理 /admin/api 到 8787）
npm run typecheck    # vue-tsc 类型检查
npm run build        # 生成静态产物到 server/adminui
```

或在仓库根执行 `./buildadmin.sh` 一键构建。

---

## 8. 提交前检查清单（本地 = CI 同等要求）

```bash
gofmt -l .        # 必须无输出
go vet ./...
go test ./...
go build ./...

cd web && npm run typecheck && npm run build   # 前端（改了 web/ 时）
```

`.github/workflows/ci.yml` 在**所有分支** push 与 PR 上运行：先构建前端 SPA
（`frontend` job），再以其产物运行 Go 的 format/vet/test/build（`test` job）。
`.github/workflows/release.yml` 在 tag push 时先构建前端，再经 `buildrelease.sh`
交叉编译，版本注入 `-X main.version=<tag>`。

**推送：**

```bash
git branch --show-current     # 必须是 beta
git add <files>
git commit -m "..."
git push origin beta          # 只推 beta
```

---

## 9. 测试指南

- 测试与被测文件同目录：`foo.go` → `foo_test.go`。
- 需覆盖的核心逻辑：路由（fallback 链、策略、别名、group、健康过滤）、
  重试/故障转移分类、限流/配额窗口、配置校验、设备鉴权。
- HTTP/转发类测试用 `httptest` 起假上游（可注入失败/延迟/SSE）。
- 时间相关逻辑用注入时钟（如 `SetNow`），**不要**用 `time.Sleep`。
- 临时产物（health/log 文件）写入临时目录，勿污染仓库。

---

## 10. 发布流程

1. 维护者切 tag（如 `vX.Y.Z`）并推送。
2. `release.yml` 枚举 `go tool dist list`，Linux/macOS runner 并行交叉编译
   （Android 下载/缓存 NDK；iOS 用 Xcode clang 输出 c-archive）。
3. 汇总产物 + `checksums.txt`，用 `softprops/action-gh-release` 发布 Release。

> 注意：发布/合并涉及 `main` 的操作只能由维护者通过受控流程执行；
> 日常开发一律停留在 `beta`（见第 0 节铁律）。

---

## 11. 修改配置 schema 的注意事项

- 新增/变更字段：同步更新 `config/config.go` 的 struct 与 `Validate()`，
  并在 `README.md` 字段表补充说明与迁移影响。
- 未讨论不得改动 `go` directive。
- 保持零第三方依赖；确需功能时优先在树内实现。

---

*本文档随开发演进修订。功能规格以 README 为准，协作规范以 CONTRIBUTING 为准。*
