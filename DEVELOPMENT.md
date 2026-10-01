# Liapi 开发文档

> 面向贡献者与维护者的开发指南。
> 功能规格与配置字段见 [README.md](README.md)；协作规范见 [CONTRIBUTING.md](CONTRIBUTING.md)；
> 安全策略见 [SECURITY.md](SECURITY.md)。

## 目录

- [0. 铁律（Must Follow）](#0-铁律must-follow)
- [1. 项目定位](#1-项目定位)
- [2. 架构总览](#2-架构总览)
- [3. 目录结构](#3-目录结构)
- [4. 核心数据流](#4-核心数据流)
- [5. 配置与热重载](#5-配置与热重载)
- [6. 安全约定](#6-安全约定不可违背)
- [7. 开发环境与常用命令](#7-开发环境与常用命令)
- [8. 管理台前端（Nuxt 4 / fuxsto-design）](#8-管理台前端nuxt-4--fuxsto-design)
- [9. 提交前检查清单（本地与 CI 同等）](#9-提交前检查清单本地与-ci-同等)
- [10. 测试指南](#10-测试指南)
- [11. CI 与发布流程](#11-ci-与发布流程)
- [12. 修改配置 schema 的注意事项](#12-修改配置-schema-的注意事项)

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

### 0.1 构建产物必须随源码提交

管理台产物 `server/adminui/` 由 `//go:embed all:adminui` 打包进二进制，**必须提交进仓库**：
CI / Docker 的 Go 构建阶段不含 Node，若产物缺失则 `go build` 直接失败。

- 改动 `web/` 后，先运行 `./buildadmin.sh` 重新生成，再连同源码一起提交。
- 不要手工编辑 `server/adminui/` 下任何文件（全是生成产物，会被下次构建覆盖）。
- `.gitignore` 只忽略 `web/node_modules`、`web/.nuxt`、`web/.output` 等中间物，
  **不**忽略 `server/adminui/`。

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
- **Go 服务**单二进制 + 内嵌管理台（`//go:embed`），`go build` 一步出产物。
- **管理台前端**为独立 Nuxt 工程（`web/`），构建产物 `server/adminui/` 再被 Go 内嵌。

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
                        │  /             内嵌管理台 SPA（Nuxt）    │
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
├── buildadmin.sh             # 构建管理台 SPA 到 server/adminui（改 web/ 后运行）
├── buildrelease.sh           # 交叉编译单个平台（release.yml 调用）
├── config/
│   ├── config.go             # Config/Upstream/Device/AliasRule/... + 默认值 + Validate + 原子 Save
│   ├── config_test.go
│   ├── holder.go             # 配置 Holder（RWMutex，热重载单一数据源）
│   └── interop.go            # OneAPI channels 导入
├── common/
│   ├── errors.go             # OpenAI 错误格式 {"error":{...}} 与写出
│   ├── token.go              # 常量时间比较 / SHA-256 / 脱敏
│   ├── password.go           # PBKDF2-HMAC-SHA256 密码哈希/校验（管理台登录）
│   └── token_test.go
├── auth/
│   ├── auth.go               # 客户端 token / 设备 token 校验
│   ├── admin.go              # 管理登录（用户名+密码→会话）/ admin token 校验 + 每 IP 失败锁定
│   └── auth_test.go
├── routing/
│   ├── router.go             # 别名解析 + 候选收集 + fallback/strategy/group/健康过滤
│   ├── router_test.go
│   └── fallback_test.go
├── relay/
│   ├── relay.go              # 转发主循环（重试 / 故障转移 / 失败分类）
│   ├── stream.go             # SSE 流式复制（Flusher）
│   ├── usage.go              # 用量抽取（OpenAI / Anthropic 两套字段）
│   ├── http.go               # 头透传、响应头清洗、尾缓冲
│   └── {relay,retry}_test.go
├── server/
│   ├── server.go             # ServeMux 路由注册 + admin 中间件（IP/锁定/限流/会话或token）
│   ├── handler_v1.go         # /v1/* 业务实现
│   ├── handler_admin.go      # /admin/api/* 实现（含 login/logout/me）+ SPA 静态托管
│   ├── requestid.go          # X-Request-ID 中间件
│   ├── adminui/              # 管理台构建产物（go:embed all:adminui），勿手改
│   └── {server,policy,failover,adminui,helpers}_test.go
├── web/                      # 管理台源码（Nuxt 4 + fuxsto-design，独立于 Go）
│   ├── nuxt.config.ts        # ssr:false + nitro.preset=static，产物输出到 ../server/adminui
│   ├── package.json          # Nuxt 4 / Vue 3 / Tailwind v4 / fuxsto-design
│   ├── tsconfig.json
│   └── app/
│       ├── app.vue           # 根组件
│       ├── layouts/default.vue  # NewAPI 风格侧边栏 + 顶栏 + 路由过渡（含未认证跳转）
│       ├── layouts/auth.vue  # 登录页专用居中布局
│       ├── pages/login.vue   # 登录页（token 校验 + 回跳 redirect）
│       ├── pages/config.vue  # 配置页（表单/JSON 双视图可视化编辑 + 热重载）
│       ├── pages/*.vue       # 概览/上游/令牌/设备/统计/日志/健康/配置/调试
│       ├── components/       # Field / DialogPanel / StatTile
│       ├── composables/      # useApi（admin token + fetch）/ useUi（toast/confirm/handleError）
│       ├── types/api.ts      # 与 Go 端 JSON 契约对应的类型
│       ├── utils/format.ts   # 数字/时间/状态格式化
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
│   ├── audit.go              # 管理端访问审计环
│   └── {stats,testhelpers}_test.go
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
- 默认数据目录 `~/.li/liapi`（`LIAPI_HOME` 可改）：`config.json`、`relay.jsonl`、
  `health_state.json` 都在那里；首次启动自动创建目录并写出 `config.json`
  （含随机 `admin_token`；**不会生成任何账号密码**——`login_enabled` 缺省即关闭）。
  权限强制 `0600`；Windows 跳过权限校验，可用 `LIAPI_SKIP_PERM_CHECK=1` 临时绕过。
  监听地址 = `addr` + `port`（`Config.ListenAddr()`；兼容旧 `addr: ":8787"` 写法），
  **改动需重启**，其余字段热重载。
- 保存流程（`config.Save`）：临时文件 → `fsync` → `os.Rename` 原子替换 → 再次 `chmod 0600`。
- 热重载：`Holder.Set(newCfg)`；所有请求**每次现取** `holder.Get()`，不缓存。
- 管理台是 Nuxt 生成的静态 SPA，产物位于 `server/adminui/`，由
  `server/server.go` 的 `//go:embed all:adminui` 打包进二进制；
  `handler_admin.go` 的 `adminUI` 负责静态资源 + SPA 回退（未知 GET 路径 → `index.html`）。
- 启动时 `main.go` 的 `printAdminBanner` 会以醒目方框打印 **管理台地址**、`admin_token`
  （开箱登录方式，供 /metrics 与脚本）以及**如何开启用户名/密码登录**的说明；
  设 `LIAPI_MASK_ADMIN_TOKEN=1` 可只打印脱敏值（共享/远程终端、录屏等场景）。
- 管理台「配置」页（`web/app/pages/config.vue`）提供**表单 / JSON 双视图**：
  - 表单视图按 Collapse 分组（监听与转发 / 路由与重试 / 健康检查 / 日志 / 管理台与鉴权 /
    别名·降级链·价目 / 告警）编辑各顶层字段；键值型字段（`aliases` / `fallbacks` /
    `prices` / `alias_rules`）用增删行编辑器维护。
  - 保存时 `buildConfig()` 以 JSON 视图为**基底**，仅覆盖表单涉及的字段，
    因此表单未覆盖的字段（`token_policies` / `token_quotas` / `upstreams` / `devices` 等）原样保留。
  - `upstreams` 与 `devices` 不在此页编辑，分别由「上游」「设备」专页维护。
  - 脱敏密钥（含 `...`）无需手填：服务端 `resolveMaskedSecrets` 会在保存时还原为原值；
    `admin_password` 留空即保持原密码，填写则更新为新密码（服务端 `Validate()` 转哈希）。
  - 登录开关 `login_enabled`：缺省（字段不存在）= 有密码哈希才开启；显式 `true` 必须已设密码，
    显式 `false` 一律拒绝密码登录（`admin_token` 不受影响）。登录页经
    `GET /admin/api/login-info`（免鉴权，只暴露两个布尔值）决定展示密码表单还是 Token 输入框。

---

## 6. 安全约定（不可违背）

1. **永不记录密钥**：token 用 `common.MaskToken` 脱敏；`api_key` 一律不入日志。
   设备 token 只存 SHA-256（`token_hash`），明文仅在创建/轮换时返回一次。
   管理台密码只存 PBKDF2-HMAC-SHA256 加盐哈希（`admin_password_hash`）；
   **任何账号密码都不自动生成**，`admin_password` 明文只写不读，`Validate()` 哈希后立即清空，
   GET/导出永不返回明文或原始哈希。
2. **常量时间比较**：token 校验用 SHA-256 摘要 + `subtle.ConstantTimeCompare`；
   密码校验用 PBKDF2 派生 + `subtle.ConstantTimeCompare`。
3. **管理面三层防护**：IP 白名单（`admin_allow_ips`）→ 每 IP 失败锁定 → 每 IP 限流
   （`admin_rate_per_minute`）→ 登录会话（用户名+密码，需 `login_enabled`）或 admin token。每次尝试写入审计环。
4. **X-Forwarded-For 仅采信可信代理**：直连对端为 loopback 或在 `admin_allow_ips` 白名单内时才读取该头，其余来源伪造 XFF 无效（防绕过 IP 白名单/锁定/限流）。
5. 日志文件 `0600`；配置文件 `0600`。
6. 前端登录会话 token 仅存浏览器 `localStorage`，随请求以 `Authorization: Bearer` 发送；
   不得写入仓库或日志。登录/退出一律走 `/login` 页面，禁止用 `window.prompt`
   等浏览器对话框收集密钥。
7. 终端打印的 `admin_token` 是唯一的开箱凭据；生产环境若会把 stdout 接入共享日志，
   应设 `LIAPI_MASK_ADMIN_TOKEN=1` 只打印脱敏值。

---

## 7. 开发环境与常用命令

```bash
git clone https://github.com/LiStudioorg/liapi.git
cd liapi
git checkout beta              # 铁律：工作在 beta 分支
go test ./...
go build -o liapi .
./liapi                        # 默认 ~/.li/liapi/config.json（自动生成，0600）
                               # 终端方框打印管理台地址与 admin_token（不生成账号密码）
```

> 启动后终端会出现一个方框，列出管理台地址与 `Admin Token` —— 开箱就用 Token 登录
> `http://<host>:8787/`。想要用户名+密码：管理台「配置 → 管理台与鉴权」打开登录开关并
> 设置密码，或直接在配置文件写 `login_enabled` + `admin_username` + `admin_password`（保存时自动转哈希）。
> 如需隐藏明文：`LIAPI_MASK_ADMIN_TOKEN=1 ./liapi -config config.json`。

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

## 8. 管理台前端（Nuxt 4 / fuxsto-design）

管理台源码在 `web/`，与 Go 代码完全解耦；**唯一契约是 `/admin/api/*` 的 JSON**。

- 技术栈：Nuxt 4（`ssr: false`）+ Vue 3 + Tailwind CSS v4 + `fuxsto-design` 组件库。
- 构建：`nuxt generate` + `nitro.preset=static`，`output.publicDir` 指向
  `../server/adminui`，产出纯静态 SPA（客户端路由）。
- 布局参考 NewApi：左侧固定侧边栏 + 顶栏，页面切换带过渡动画
  （`pageTransition`，`mode: out-in`）。
- **登录**：`app/pages/login.vue` 是独立的登录页，配 `layouts/auth.vue`（居中品牌布局）。
  - 输入**用户名 + 密码**，调用 `/admin/api/login` 换取短期会话 token（`sess-…`）。
  - 未认证（`localStorage` 无会话 token）访问任意页面 → 自动跳转 `/login?redirect=<原路径>`。
  - 登录成功才写入 `localStorage`（错误凭据不会覆盖已有会话），再回跳 `redirect`；密码框提供显示/隐藏切换。
  - 任意接口返回 401 → `useUi.handleError` 清除无效态并跳转 `/login`。
  - 退出登录调用 `/admin/api/logout` 使服务端会话失效后再清除本地 token。
  - **禁止**使用 `window.prompt` / `window.confirm` 等浏览器对话框收集或确认密钥。
- API 客户端：`app/composables/useApi.ts`——会话 token 存 `localStorage`，
  请求带 `Authorization: Bearer <session>`；`login()` 换 token、`logout()` 服务端失效并登出。
- 类型契约：`app/types/api.ts`，字段须与 `server/handler_admin.go` 保持一致。
- 开发代理：`nuxt.config.ts` 的 `nitro.devProxy` 已把 `/admin/api`、`/v1`、`/metrics`
  转发到本地 `127.0.0.1:8787`，可先起 Go 服务再 `npm run dev`。

### 8.1 配置页可视化编辑（`app/pages/config.vue`）

配置页是「可视化编辑 + 热重载」的统一入口，控件与 `config/config.go` 字段一一对应：

- 顶部 `Segmented` 切换 **表单 / JSON** 两个视图，共用同一份 `text`（GET `/admin/api/config` 返回的脱敏 JSON）。
- 表单字段用 `reactive(form)` 承载标量项，用 `ref([])` 承载键值/列表项；
  `readForm(cfg)` 把配置铺进表单，`buildConfig()` 反向合成。
- **合成策略**：`buildConfig()` 先解析 JSON 视图为基底对象，再 `Object.assign` 覆盖表单字段。
  这样新增字段只要没加进表单就会自动透传，避免「表单漏字段导致配置丢失」。
- 保存仍走 `POST /admin/api/config` 单一接口，服务端做 `Validate` → `Save`（原子）→ `Holder.Set`；
  校验失败保留旧配置，前端提示错误。
- 导出的 JSON（`/config/export`）含明文密钥，仅用于备份，切勿提交仓库。

> 新增配置字段时（见第 12 节）：除更新 `config/config.go` 与 README 字段表外，
> 若希望出现在管理台表单中，还需同步 `web/app/types/api.ts` 类型与本页控件。

常用命令（在 `web/` 内）：

```bash
npm install          # 安装依赖（首次）
npm run dev          # 开发服务器（默认 3000，已代理 /admin/api 到 8787）
npm run typecheck    # vue-tsc 类型检查
npm run build        # 生成静态产物到 server/adminui
```

或在仓库根执行 `./buildadmin.sh` 一键构建（缺依赖会先 `npm ci`）。

### 8.2 构建产物生命周期

```
web/app/**  ──npm run build──►  server/adminui/**  ──go:embed──►  二进制
   (源码，提交)                     (产物，提交)                    (go build)
```

- 两者都提交；CI 会在 Go 构建前重新生成产物并覆盖，确保校验的是最新源码。
- 本地只需保证「改了 `web/` 就一定跑过 `./buildadmin.sh` 并提交产物」。
- 排查页面 404/资源 404：确认 `server/adminui/index.html` 与 `_nuxt/` 已存在且被提交。

---

## 9. 提交前检查清单（本地与 CI 同等）

```bash
gofmt -l .        # 必须无输出（注意：仓库统一 CRLF，Linux CI 下为 LF）
go vet ./...
go test ./...
go build ./...

cd web && npm run typecheck && npm run build   # 前端（改了 web/ 时）
```

> Windows 本地 `go test ./...` 会因文件权限校验（`TestLoadPermCheck` 等）失败，
> 这是平台差异（`chmod` 在 Windows 为 no-op）；Linux CI 下正常。可用
> `LIAPI_SKIP_PERM_CHECK=1` 跳过相关校验后在本地跑其余测试。

**推送：**

```bash
git branch --show-current     # 必须是 beta
git add <files>
git commit -m "..."
git push origin beta          # 只推 beta
```

---

## 10. 测试指南

- 测试与被测文件同目录：`foo.go` → `foo_test.go`。
- 需覆盖的核心逻辑：路由（fallback 链、策略、别名、group、健康过滤）、
  重试/故障转移分类、限流/配额窗口、配置校验、设备鉴权、管理台 SPA 托管。
- HTTP/转发类测试用 `httptest` 起假上游（可注入失败/延迟/SSE）。
- 时间相关逻辑用注入时钟（如 `SetNow`），**不要**用 `time.Sleep`。
- 临时产物（health/log 文件）写入临时目录（`t.TempDir()`），勿污染仓库。
- 共享测试桩在 `server/helpers_test.go`、`stats/testhelpers_test.go`：
  `newTestServer` 装配完整 Server，`serve`/`do` 发送请求，`newOKUpstream` 起假上游。

---

## 11. CI 与发布流程

### 11.1 CI（`.github/workflows/ci.yml`）

在所有分支 push 与 PR 上运行两个 job：

1. `frontend`（Node 22）：`npm ci` → `npm run typecheck` → `npm run build`，
   上传 `server/adminui` 为 artifact。
2. `test`（Go stable，`needs: frontend`）：下载 artifact 覆盖 `server/adminui`，
   再执行 `gofmt -l .` / `go vet` / `go test` / `go build`，确保内嵌的是最新产物。

### 11.2 自动发布（`publish` job，仅 beta）

- 触发条件：`push` 且 `ref == refs/heads/beta`，且 `frontend` + `test` 均通过。
- 行为：交叉编译 6 个常用平台（linux/windows/darwin × amd64/arm64），
  版本号注入 `beta-<sha>`，**覆盖式**发布到固定 tag `beta-latest` 的**正式 Release**。
- 每次发布前清空旧资产，保证 `beta-latest` 只保留最新提交的产物 + `checksums.txt`。
- `make_latest: false`：不抢占版本化 Release 的 "Latest" 徽章。

### 11.3 版本化发布（`.github/workflows/release.yml`，打 tag）

1. 维护者切 tag（如 `vX.Y.Z`）并推送。
2. `release.yml` 先构建前端，再枚举 `go tool dist list`，Linux/macOS runner 并行
   交叉编译（Android 下载/缓存 NDK；iOS 用 Xcode clang 输出 c-archive）。
3. 汇总产物 + `checksums.txt`，用 `softprops/action-gh-release` 发布 Release，
   版本注入 `-X main.version=<tag>`。

> 发布/合并涉及 `main` 的操作只能由维护者通过受控流程执行；
> 日常开发一律停留在 `beta`（见第 0 节铁律）。

---

## 12. 修改配置 schema 的注意事项

- 新增/变更字段：同步更新 `config/config.go` 的 struct 与 `Validate()`，
  并在 `README.md` 字段表补充说明与迁移影响。
- 若字段出现在管理台表单：同步更新 `web/app/types/api.ts`、`web/app/pages/config.vue`
  的 `form`/`readForm()`/`buildConfig()`（或对应专页组件）。
- 兼容旧配置：`buildConfig()` 以 JSON 视图为基底，未加入表单的新字段会透传，
  不会因表单遗漏而被抹掉；但仍应把常用字段补进表单以便可视化编辑。
- 密钥类字段需在 `server/handler_admin.go` 的 `adminGetConfig` 脱敏、在
  `resolveMaskedSecrets` 还原（如 `admin_password_hash` 已在两处处理）。
- 未讨论不得改动 `go` directive。
- 保持零第三方依赖（Go 侧）；确需功能时优先在树内实现。

---

*本文档随开发演进修订。功能规格以 README 为准，协作规范以 CONTRIBUTING 为准。*
