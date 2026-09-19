# Liapi — OpenAI 兼容 API 中转站

> 按 P0 → P1 → P2 顺序实现。P0 完成即可作为可用的中转站使用。

---

## 一、项目简介

一个跑在自己机器/服务器上的 HTTP 服务，对外暴露 OpenAI 兼容 API，对内聚合多个上游（OpenAI、Claude、OpenRouter、硅基流动、Ollama…），统一 **鉴权、路由、日志、限流、故障转移**。

技术栈：**Go 1.22+ / 纯标准库**（无第三方依赖，`go build` 一步出二进制）。

- 不是一个多租户 SaaS：不做注册、支付、发票。
- 不做模型推理本身。

---

## 二、边界定义

| 是什么 | 不是什么 |
|---|---|
| 自托管 HTTP 服务 | 多租户 SaaS |
| OpenAI 兼容网关 | 不做注册/支付/发票 |
| 统一鉴权 + 路由 + 限流 + 日志 | 不做模型推理 |
| 支持流式 / 故障转移 | （P2）不强制做多用户配额 |

---

## 三、架构总览

```
                     ┌───────────────────────────────┐
                     │            liapi              │
                     │                               │
  客户端 ──────────► │  /v1/*     业务入口（OpenAI 兼容）│
  (SDK/base_url)     │   ├─ 鉴权（常量时间比较）        │
                     │   ├─ 限流（固定窗口）           │
                     │   ├─ 解析 body / model          │
                     │   ├─ 模型路由（priority+weight）│
                     │   ├─ 转发 + 故障转移（重试）     │
                     │   │      ├─ 流式 / 非流式        │
                     │   │      └─ 用量统计             │
                     │   └─ 日志（JSONL + 环形缓冲）    │
                     │                               │
                     │  /admin/api/*  管理接口         │
                     │  /admin        内嵌管理台(单文件)│
                     │                               │
                     │  后台：健康检查 goroutine        │
                     │       环形日志缓冲 / 用量聚合    │
                     └───────────────────────────────┘
                        │          │           │
                 ┌──────┴───┐ ┌────┴────┐ ┌────┴─────┐
                 │ OpenAI   │ │ OpenRouter│ │ Ollama   │ ...
                 └──────────┘ └─────────┘ └──────────┘
                   上游 A      上游 B/(同层可加权负载均衡)
```

**关键设计**：

1. 上游 `base_url` 直接带版本前缀（如 `https://api.openai.com/v1`），转发时只做 `base_url`（去尾斜杠）+ 去 `/v1` 前缀后的请求路径拼接，不硬编码路径 —— 各家厂商路径各不相同。
2. 客户端 token 与上游 API Key **完全分离**：前者来自 `config.client_tokens`，后者只属于出站请求。
3. 故障转移决策必须在**第一次 WriteHeader 之前**完成：只为「已写响应头」的请求打日志钩子，切换发生在转发循环内。
4. 流式转发**不设 `http.Client.Timeout`**（会掐断长流），用 `context.WithTimeout` 控制非流式；流式靠客户端断开驱动。

---

## 四、目录结构

```
liapi/
├── go.mod
├── main.go                  # 入口：加载配置、装配、启动 HTTP、优雅退出
├── config/
│   ├── config.go            # Config / Upstream 结构、默认值、Validate
│   └── holder.go            # 配置 Holder（读写锁，热重载的单一数据源）
├── common/
│   ├── errors.go            # OpenAI 错误格式 {"error":{...}} 与 HTTP 写入
│   └── token.go             # 常量时间比较 / token 脱敏
├── auth/
│   ├── auth.go              # 客户端 token 校验（Bearer / x-api-key / ?token=）
│   └── admin.go             # 管理 token 校验（独立）
├── routing/
│   ├── router.go            # 候选收集 → priority 升序 → 组内加权随机打散
│   └── aliases.go           # （P2）模型别名 / 参数覆写
├── relay/
│   ├── relay.go             # 转发主循环（含故障转移 / 重试）
│   ├── stream.go            # SSE 流式复制（32KB 缓冲 + Flusher）
│   ├── usage.go             # 用量抽取（prompt/input-* 兼容两套字段）
│   └── http.go              # 透传头、响应头清洗、尾 64KB 缓冲
├── server/
│   ├── server.go            # ServeMux 路由注册（Go 1.22 方法路由）
│   ├── handler_v1.go        # /v1/chat/completions、completions、embeddings、
│   │                        #   messages、models 的实现
│   └── handler_admin.go     # /admin/api/*
├── adminui/
│   └── index.html           # 单文件管理台（go:embed 打包）
├── stats/
│   ├── logger.go            # JSONL 写文件 + 环形缓冲（数组+取模）
│   ├── limiter.go           # 固定窗口限流 map[token]{count,reset} + 定期清理
│   ├── health.go            # 后台探测 goroutine，内存 map 存状态
│   └── totals.go            # 全量计数/用量/费用的原子聚合
└── README.md
```

---

## 五、配置文件设计

默认读取 `./config.json`，可用 `-config <path>` 指定。

```jsonc
{
  "addr": ":8787",                        // 监听地址
  "body_limit_bytes": 16777216,           // 请求体上限，默认 16MB
  "timeout": 300,                         // 非流式转发超时（秒），0=关闭
  "stream_timeout": 0,                    // 流式超时（秒），0=不设
  "max_idle_conns": 100,                  // 连接复用池
  "log_file": "relay.jsonl",              // JSONL 日志文件
  "ring_size": 800,                       // 内存环形缓冲条数
  "probe_interval": 30,                   // 健康检查间隔（秒）
  "probe_fail_threshold": 3,              // 连续失败 N 次才判不健康
  "skip_unhealthy": true,                 // 路由时避开不健康上游（可开关）

  "admin_token": "adm-sk-xxxxxxxx",       // 管理接口鉴权（独立）
  "client_tokens": ["sk-client-aaa", ...],// 合法的客户端 token 数组

  "rate_limit_per_minute": 0,             // 每 token 每分钟请求数，0=关闭

  "aliases": {                            // (P2) 模型别名
    "gpt-4o": "openai/gpt-4o"
  },

  "prices": {                             // (P2) 费用估算，元/1M tokens
    "gpt-4o":        { "input": 20,  "output": 60  },
    "claude-3-5-sonnet": { "input": 20, "output": 100 }
  },

  "upstreams": [
    {
      "name": "openai-main",
      "base_url": "https://api.openai.com/v1",
      "api_key": "sk-up-a",
      "models": ["gpt-4o", "gpt-4o-mini"],
      "priority": 1,                 // 数字越小越优先
      "weight": 1,                   // 同优先级内加权随机
      "disabled": false,             // 临时停用
      "health_path": "/v1/models",   // 健康检查路径（默认）
      "retry": 1,                    // 单上游内部重试次数
      "inject_usage": false          // 流式时注入 stream_options.include_usage
    },
    {
      "name": "anthropic-main",
      "base_url": "https://api.anthropic.com/v1",
      "api_key": "sk-ant-up-b",
      "models": ["claude-3-5-sonnet"],
      "priority": 1,
      "weight": 1
    },
    {
      "name": "local-ollama",
      "base_url": "http://localhost:11434/v1",
      "api_key": "",                 // 本地无鉴权
      "models": ["llama3.1", "qwen2.5"]
    },
    {
      "name": "openrouter-fallback",
      "base_url": "https://openrouter.ai/api/v1",
      "api_key": "sk-or-...",
      "models": ["*"],               // 兜底：任意 model 都匹配
      "priority": 9
    }
  ]
}
```

### 字段说明

| 字段 | 必填 | 说明 |
|---|---|---|
| `name` | ✅ | 唯一标识，日志/健康检查用 |
| `base_url` | ✅ | **带版本前缀**（如 `…/v1`），不带尾斜杠 |
| `api_key` | ✅ | 上游密钥，仅出站注入 |
| `models` | ✅ | 支持的模型列表，`"*"` 兜底 |
| `priority` | 建议 | 越小越优先 |
| `weight` | 可选 | 同优先级内加权随机，默认 1 |
| `disabled` | 可选 | 临时停用 |
| `health_path` | 可选 | 健康检查路径，默认 `/v1/models` |
| `retry` | 可选 | 单上游内部重试次数 |
| `inject_usage` | 可选 | 流式时写入 `stream_options: {include_usage:true}` |

> **为什么 `base_url` 带 `/v1`**：不同厂商前缀不一致（OpenAI `/v1`、Anthropic `/v1`、部分自定义网关无前缀）。统一规则：客户端路径去 `/v1` 前缀 + 上游 `base_url` 拼接，路径全部由一个规则产生，可预测、可 debug。

---

## 六、核心流程

### 6.1 请求处理链路（以 `POST /v1/chat/completions` 为例）

```
r.Context()
  ├─ 鉴权：从 Authorization / x-api-key / ?token= 取 token
  │       常量时间比较 client_tokens ── 失败 → 401（OpenAI 错误格式）
  ├─ 限流：limiter.Allow(token) ── 超限 → 429
  ├─ 读 body（MaxBytesReader，16MB）── 超限 → 413；非法 JSON → 400
  ├─ 解析 model / stream ── 缺 model → 400
  ├─ 别名解析（P2）＋ 参数覆写（P2）→ 得到最终 model
  ├─ 路由：candidates = 匹配上游
  │       priority 升序 → 同 priority 组内 weight 加权随机打散
  ├─ 转发主循环（见 6.3，含故障转移）
  ├─ 响应回写：SSE 流式 / 普通复制（清洗响应头）
  ├─ 用量抽取（usage）
  └─ 写日志（JSONL 后台队列 + 环形缓冲 + 全量计数）
```

### 6.2 鉴权细节

- 读取顺序：`Authorization: Bearer <t>` → `x-api-key: <t>` → `?token=<t>`。
- `Bearer` 前缀**大小写不敏感**。
- 比较用 **SHA-256 摘要 + `crypto/subtle.ConstantTimeCompare`**，避免时序攻击与长度侧信道。
- token 脱敏：`sk-abc12345xyz` → `sk-ab...xyz` 后再进日志。
- 空 token / 格式错误 → `401`，绝不 `500`。

### 6.3 转发主循环（含故障转移）

```text
candidates = 路由结果 [A, B, C]

for i, up := range candidates:
    for attempt := 0; attempt <= up.retry; attempt++:
        # 构造出站请求：注入 Authorization / x-api-key，Content-Type
        # 非流式：context.WithTimeout(timeout)；流式：不设硬超时
        resp, err := client.Do(req)
        if err != nil:                       # 连接失败/超时 → 可切换
            log(attempt err); continue
        if 2xx:                              # 命中 → 回写并返回
            return copyResponse(w, resp)
        snippet = readAtMost(resp.Body, 8KB) # 读数用于日志
        if 4xx && status != 429:             # 请求本身有问题 → 不再换
            return failFast(up, status, snippet)
        # 429 / 5xx → 可切换，继续下一个 attempt/upstream
return failAll(lastErr)                       # 502 或透传最后一次的 status
```

**切换前提**：回写前完成全部决策；一旦 `WriteHeader` 被执行，不再切换。

### 6.4 流式转发（SSE）

- 用上游响应头 `Content-Type` 是否含 `text/event-stream` 判断是否流式。
- `bufio.Reader` 逐行读，每行写出后调用 `http.Flusher.Flush()`。
- 复用 `http.Client`（`Transport.MaxIdleConns`）。
- 客户端断开（`Write` 报错）→ 立即停止读上游、`Close()` 上游 body 释放连接。
- 逐行尝试解析 `data:` 里的 JSON，抽取 `usage`（`prompt_tokens/completion_tokens` 或 `input_tokens/output_tokens`），流式最后一帧通常带 usage。
- 响应头清洗：**删除 `Content-Length / Transfer-Encoding / Connection`**，其余透传（含 `Content-Type`）。

### 6.5 用量统计

| 来源 | 处理方式 |
|---|---|
| 非流式 | 响应复制时同时写入「尾 64KB 滑动缓冲」，请求结束后扫描 `"usage"` JSON（兼容 prompt/input 两套字段） |
| 流式 | 逐帧解析 SSE data，命中即记录 |
| 兼容字段 | OpenAI：`prompt_tokens/completion_tokens`；Anthropic：`input_tokens/output_tokens` |

聚合维度：总量 / 按上游 / 按模型 / 按 token / 按时间段。

### 6.6 日志

- **路径**：追加写 JSONL 文件（一行一条，可 grep/jq）。
- **内存**：环形缓冲（固定数组 + 取模，`ring_size` 条），供管理台查询。
- **并发**：所有写入走同一个后台 goroutine 队列（`select + default` 丢弃防阻塞），单点加锁。
- **脱敏**：`token` 用 `sk-ab...xyz`，上游 `api_key` 一律不落日志。

一条日志字段：

```json
{"time":"…","token":"sk-ab…xyz","path":"/v1/chat/completions","model":"gpt-4o",
 "upstream":"openai-main","status":200,"stream":true,"latency_ms":234,
 "in_tokens":12,"out_tokens":340,"cost":0.000201,"error":""}
```

---

## 七、API 规格

### 7.1 业务 API（同 OpenAI，客户端零改动切 base_url）

| 路径 | 方法 | 说明 |
|---|---|---|
| `/v1/chat/completions` | POST | 主力聊天补全 |
| `/v1/completions` | POST | 老式补全 |
| `/v1/embeddings` | POST | 向量 |
| `/v1/models` | GET | 列出可用模型（汇总各上游 + 别名） |
| `/v1/messages` | POST | Claude 原生格式（接 Anthropic 时） |

**约束**：

- 出站请求体 = 客户端请求体原样（无别名/覆写时不做任何二次序列化，保证保真）。
- 错误一律 OpenAI 格式，避免客户端 SDK 解析崩：

```json
{ "error": { "message": "...", "type": "...", "code": "..." } }
```

- `413` 请求体超限；`400` 非法 JSON / 缺 model；`401` 鉴权失败；`429` 限流；`502` 无上游匹配或全部失败。

### 7.2 管理 API（独立 admin token）

| 路径 | 方法 | 说明 |
|---|---|---|
| `/admin` | GET | 管理台单页（go:embed） |
| `/admin/api/overview` | GET | 请求数 / 成功率 / 平均延迟 / token 量 / 费用 |
| `/admin/api/upstreams` | GET | 上游列表（api_key 脱敏） |
| `/admin/api/upstreams` | POST | 新增上游 |
| `/admin/api/upstreams/{name}` | PUT | 更新上游（api_key 留空=保持原值） |
| `/admin/api/upstreams/{name}` | DELETE | 删除上游 |
| `/admin/api/tokens` | GET/POST | 列出 / 新增客户端 token |
| `/admin/api/tokens/delete` | POST | 删除 token（传全文） |
| `/admin/api/logs?n=` | GET | 最近 N 条日志 |
| `/admin/api/health` | GET | 上游健康状态 |
| `/admin/api/test` | POST | 调试：{model, messages, stream} 直接发一条 |

管理台前端把 admin token 存 `localStorage`，每次请求带 `Authorization: Bearer <admin_token>`。**管理 token 与客户端 token 严格分离**。

---

## 八、P0 方案（第 1–2 天）

### 里程碑 P0-1：骨架 + 非流式链路

1. `config`：结构体 / 默认值 / `Validate()` / `Load` / `Holder`。
2. `common`：OpenAI 错误写入、常量时间比较、token 脱敏。
3. `auth`：客户端 token 校验（三来源，Bearer 大小写不敏感）。
4. `routing`：候选收集 → priority 升序 → 组内加权随机（展开为数组 → Fisher-Yates 洗牌 → 去重保序）。
5. `relay`：非流式转发 —— 头注入 / 响应头清洗 / `200~299` 回写 / 非 2xx 分段读 body。
6. `server.handler_v1`：5 条业务路径。

### 里程碑 P0-2：流式 + 故障转移 + 日志

1. SSE 流式复制（`text/event-stream` 检测、逐行 Flush、客户端断开停止读）。
2. 故障转移循环 + 单上游重试 + `4xx(≠429)` 快速失败。
3. 用量抽取（尾 64KB + SSE 逐帧）。
4. `stats.logger`：JSONL + 环形缓冲 + 队列 goroutine。

**P0 验收清单**

- [ ] curl 直连 `/v1/chat/completions` 调 OpenAI 成功（非流式 + 流式）。
- [ ] 拔掉 A 上游 key → 自动走 B（故障转移）。
- [ ] 错误格式是 `{"error":{...}}`；缺 model=400、坏 token=401、超大 body=413。
- [ ] 日志文件与环形缓冲数据一致且 token 已脱敏。

---

## 九、P1 方案（第 3–4 天）

### P1-8 管理台

单 HTML（CSS+JS 内联），`go:embed` 进二进制，零构建。Tabs：概览 / 上游 / 令牌 / 日志 / 调试 / 健康。

### P1-9 配置热重载

- 所有请求从 `Holder.Get()` 现取，不缓存。
- 管理台保存：`Validator()` 校验通过 → 写临时文件 → `os.Rename` 原子覆盖 → `Holder.Set(newCfg)`。
- 不做 fsnotify，自用场景管理台保存即热更。

### P1-10 健康检查

后台 goroutine 每 `probe_interval` 并发探测所有启用上游（`GET health_path` + Authorization，5s 超时）。连续失败 `probe_fail_threshold` 次才标不健康；`skip_unhealthy=true` 时路由避开，若过滤后为空则回退全量（防止全挂时完全不可用）。

### P1-11 限流

固定窗口：`map[token]{count, resetTime}`，`limit=0` 关闭，超限 `429`。后台每 60s 清理过期桶防内存增长。**位置：鉴权后、转发前**。

### P1-12 用量统计

见 6.5。`totals` 累加请求数 / 成功数 / 延迟总和 / in、out tokens / 费用（P2 价格）。

### P1-13 多上游负载均衡

同 `priority` 组内按 `weight` 加权随机（洗牌去重保序），不引入轮询计数器，避免重启丢状态。

---

## 十、P2 加分项（视时间挑选）

| # | 功能 | 要点 |
|---|---|---|
| 14 | 费用估算 | `cost = in/1e6*priceIn + out/1e6*priceOut`，按模型配置价 |
| 15 | 模型别名 | `aliases` map，转发前重写请求体 `model` 字段 |
| 16 | 参数覆写 | 按上游/模型 force/remove 参数 |
| 17 | 响应缓存 | key=`hash(model+messages+关键参数)`，仅非流式且非 temperature>0，内存 LRU |
| 18 | 请求钩子 | 前置/后置插件式接口 |
| 19 | 告警 | 连续失败 N 次 / 费用超阈值 → Webhook POST（飞书/钉钉/Telegram） |
| 20 | 多用户 / 配额 | 每 token 关联用户 + 月度配额，超限 429（偏 SaaS） |

---

## 十一、实现顺序

```
第 1 天：P0-1 入口 + 鉴权 + 上游配置 + 路由 + 非流式转发
第 2 天：P0-2 流式 + 故障转移 + 日志
第 3 天：P1-8 管理台 + P1-9 热重载
第 4 天：P1-10 健康检查 + P1-11 限流 + P1-12 用量
第 5 天：P1-13 负载均衡 + P2 挑 2–3 个
```

每日产出均需 `go build ./... && go vet ./...` 通过，并有 curl 冒烟用例。

---

## 十二、关键技术决策与坑位规避

| # | 坑 | 对策 |
|---|---|---|
| 1 | `http.Client.Timeout` 会掐断流式 | 非流式用 `context.WithTimeout`；流式不设硬超时，靠客户端断开 |
| 2 | 透传 `Content-Length` 导致流式 HTTP 层错乱 | 响应头显式删除 `Content-Length / Transfer-Encoding / Connection` |
| 3 | 写过响应头就不能换上游 | 切换决策全部在首次 `WriteHeader` 前完成 |
| 4 | 配置写到一半崩溃 → 文件损坏 | 临时文件 + `os.Rename` 原子替换 |
| 5 | 比较 token 有时序攻击面 | SHA-256 摘要 + `subtle.ConstantTimeCompare` |
| 6 | 日志锁粒度过大 | JSONL 写入收敛到单一后台 goroutine，请求路径只投递不等待 |
| 7 | 环形缓冲用 slice append+trim 会涨内存 | 固定数组 + head 指针 + 取模 |
| 8 | 上游 401 去重试 | 4xx（除 429）判定为请求自身问题，直接失败返回 |
| 9 | 流式 usage 在最后一帧，靠前取不到 | SSE 逐帧解析 + 非流式尾 64KB 缓冲兜底 |
| 10 | 上游 key 打进日志 | 出错仅打 URL + 状态码，`api_key` 永不落日志 |
| 11 | 非法 JSON 位置难定位 | 报错带畸形 JSON 的起止位置（`json.SyntaxError.Offset`） |
| 12 | 客户端中途断开 panic | 复制循环中 `Write` 出错即退出，不访问已关闭的连接 |

---

## 十三、测试方案

1. **单元**：`routing`（优先级/权重随机性、`*` 兜底、全 disabled）、`auth`（三来源、大小写、空值）、`limiter`（窗口/清理）、`config.Validate`。
2. **集成**：用 `httptest` 起假上游（可注入失败/延迟/SSE），跑转发 + 故障转移 + 用量。
3. **冒烟**：curl 脚本覆盖 P0 验收清单（正常/流式/切换/错误格式）。
4. **性能**：`MaxIdleConns` 复用验证（连接不重建），并发 100 请求压测无连接泄漏。

---

## 十四、运行方式

```bash
# 首次运行自动生成默认 config.json（含随机 admin_token，打印到控制台）
go build -o liapi ./...
./liapi -config config.json

# 冒烟
curl http://localhost:8787/v1/models -H "Authorization: Bearer sk-client-aaa"
curl http://localhost:8787/v1/chat/completions -H "Authorization: Bearer sk-client-aaa" \
     -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}' -N

# 管理台
open http://localhost:8787/admin
```

---

## 十五、文件清单（实现后应有）

```
main.go                     server/handler_v1.go      routing/router.go
config/config.go            server/handler_admin.go   routing/aliases.go
config/holder.go            server/server.go          relay/relay.go
auth/auth.go                stats/logger.go           relay/stream.go
auth/admin.go               stats/limiter.go          relay/usage.go
common/errors.go            stats/health.go           relay/http.go
common/token.go             stats/totals.go           adminui/index.html
```

---

*本 README 即实现方案（spec），实现顺序与决策可随开发演进修订。*