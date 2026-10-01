<script setup lang="ts">
import {
  Card,
  Button,
  Input,
  InputNumber,
  Switch,
  Segmented,
  Textarea,
  Alert,
  Collapse,
  CollapseItem,
} from 'fuxsto-design'
import {
  RefreshCw,
  Save,
  Download,
  Upload,
  FileJson,
  Plus,
  Trash2,
  Eye,
  Code2,
} from 'lucide-vue-next'
import type { Price } from '~/types/api'

const { request, download } = useApi()
const { toast, confirm, handleError } = useUi()

const text = ref('')
const saving = ref(false)
const loading = ref(true)
const dirty = ref(false)
const view = ref<'form' | 'json'>('form')
const openPanels = ref<(string | number)[]>(['network'])

// Editable view of the config. Only fields surfaced by the API (masked JSON
// round-trip) are represented; upstreams/devices keep their dedicated pages.
const form = reactive({
  addr: '',
  port: 0,
  body_limit_bytes: 0,
  timeout: 0,
  stream_timeout: 0,
  stream_idle_timeout: 0,
  max_idle_conns: 0,
  probe_interval: 0,
  probe_timeout: 0,
  probe_fail_threshold: 0,
  probe_concurrency: 0,
  skip_unhealthy: false,
  log_file: '',
  log_max_bytes: 0,
  log_retention_days: 0,
  log_raw_tokens: false,
  ring_size: 0,
  retry_on_timeout: false,
  rate_limit_per_minute: 0,
  daily_per_token: 0,
  admin_username: '',
  admin_token: '',
  admin_password: '',
  login_enabled: false,
  admin_rate_per_minute: 0,
  admin_allow_ips: '',
  metrics_token: '',
  route_strategy: 'priority',
  alerts: {
    webhooks: [] as string[],
    bark: [] as string[],
    on_failover: false,
    on_unhealthy: false,
    on_quota: false,
    quota_warn_percent: 80,
    min_alert_interval_seconds: 60,
  },
})

// Structured key/value editors serialized back to the raw JSON on save.
const aliases = ref<{ k: string; v: string }[]>([])
const fallbacks = ref<{ model: string; chain: string }[]>([])
const prices = ref<{ model: string; input: number; output: number }[]>([])
const aliasRules = ref<
  { pattern: string; regex: boolean; model: string; params: string }[]
>([])
const routeOptions = [
  { label: 'priority（优先级+权重）', value: 'priority' },
  { label: 'latency（按延迟）', value: 'latency' },
  { label: 'cost（按价目）', value: 'cost' },
]


function prettyBytes(n: number): string {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${Number(v.toFixed(v < 10 ? 2 : 1))} ${units[i]}`
}

type Raw = Record<string, any>

function readForm(cfg: Raw) {
  const a = (cfg.alerts || {}) as {
    webhooks?: string[]
    bark?: string[]
    on_failover?: boolean
    on_unhealthy?: boolean
    on_quota?: boolean
    quota_warn_percent?: number
    min_alert_interval_seconds?: number
  }
  Object.assign(form, {
    addr: cfg.addr ?? '',
    body_limit_bytes: Number(cfg.body_limit_bytes) || 0,
    timeout: Number(cfg.timeout) || 0,
    stream_timeout: Number(cfg.stream_timeout) || 0,
    stream_idle_timeout: Number(cfg.stream_idle_timeout) || 0,
    max_idle_conns: Number(cfg.max_idle_conns) || 0,
    probe_interval: Number(cfg.probe_interval) || 0,
    probe_timeout: Number(cfg.probe_timeout) || 0,
    probe_fail_threshold: Number(cfg.probe_fail_threshold) || 0,
    probe_concurrency: Number(cfg.probe_concurrency) || 0,
    skip_unhealthy: !!cfg.skip_unhealthy,
    log_file: cfg.log_file ?? '',
    log_max_bytes: Number(cfg.log_max_bytes) || 0,
    log_retention_days: Number(cfg.log_retention_days) || 0,
    log_raw_tokens: !!cfg.log_raw_tokens,
    ring_size: Number(cfg.ring_size) || 0,
    retry_on_timeout: !!cfg.retry_on_timeout,
    rate_limit_per_minute: Number(cfg.rate_limit_per_minute) || 0,
    daily_per_token: Number(cfg.daily_per_token) || 0,
    admin_username: cfg.admin_username ?? '',
    admin_token: cfg.admin_token ?? '',
    login_enabled:
      typeof cfg.login_enabled === 'boolean'
        ? cfg.login_enabled
        : !!cfg.admin_password_hash,
    admin_rate_per_minute: Number(cfg.admin_rate_per_minute) || 0,
    admin_allow_ips: (cfg.admin_allow_ips || []).join(', '),
    metrics_token: cfg.metrics_token ?? '',
    route_strategy: cfg.route_strategy || 'priority',
  })
  form.port = Number(cfg.port) || 0
  form.admin_password = ''
  form.alerts = {
    webhooks: a.webhooks || [],
    bark: a.bark || [],
    on_failover: !!a.on_failover,
    on_unhealthy: !!a.on_unhealthy,
    on_quota: !!a.on_quota,
    quota_warn_percent: Number(a.quota_warn_percent ?? 80),
    min_alert_interval_seconds: Number(a.min_alert_interval_seconds ?? 60),
  }
  aliases.value = Object.entries((cfg.aliases || {}) as Record<string, string>).map(
    ([k, v]) => ({ k, v }),
  )
  fallbacks.value = Object.entries(
    (cfg.fallbacks || {}) as Record<string, string[]>,
  ).map(([model, chain]) => ({ model, chain: (chain || []).join(', ') }))
  prices.value = Object.entries((cfg.prices || {}) as Record<string, Price>).map(
    ([model, p]) => ({ model, input: p.input ?? 0, output: p.output ?? 0 }),
  )
  aliasRules.value = ((cfg.alias_rules || []) as Raw[]).map((r) => ({
    pattern: r.pattern ?? '',
    regex: !!r.regex,
    model: r.model ?? '',
    params: r.params ? JSON.stringify(r.params) : '',
  }))
}

async function load() {
  loading.value = true
  try {
    const cfg = await request<Raw>('/config')
    text.value = JSON.stringify(cfg, null, 2)
    readForm(cfg)
    dirty.value = false
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

/** Compose the full config payload: structured edits overlay the raw JSON, so
 *  fields without a form control (token_policies, token_quotas, …) survive. */
function buildConfig(): Raw {
  let base: Raw
  try {
    base = JSON.parse(text.value) as Raw
  } catch (e) {
    throw new Error('JSON 视图不是合法 JSON: ' + (e as Error).message)
  }

  const aliasesObj: Record<string, string> = {}
  for (const { k, v } of aliases.value) {
    if (k.trim()) aliasesObj[k.trim()] = v
  }
  const fallbacksObj: Record<string, string[]> = {}
  for (const { model, chain } of fallbacks.value) {
    const names = chain.split(',').map((s) => s.trim()).filter(Boolean)
    if (model.trim() && names.length) fallbacksObj[model.trim()] = names
  }
  const pricesObj: Record<string, Price> = {}
  for (const p of prices.value) {
    if (p.model.trim())
      pricesObj[p.model.trim()] = {
        input: Number(p.input) || 0,
        output: Number(p.output) || 0,
      }
  }
  const rules = []
  for (const r of aliasRules.value) {
    if (!r.pattern.trim() || !r.model.trim()) continue
    const rule: Raw = {
      pattern: r.pattern.trim(),
      regex: r.regex,
      model: r.model.trim(),
    }
    if (r.params.trim()) {
      try {
        rule.params = JSON.parse(r.params)
      } catch {
        throw new Error(`alias_rules[${r.pattern}] params 不是合法 JSON`)
      }
    }
    rules.push(rule)
  }

  Object.assign(base, {
    addr: form.addr,
    port: Number(form.port) || 0,
    body_limit_bytes: Number(form.body_limit_bytes) || 0,
    timeout: Number(form.timeout) || 0,
    stream_timeout: Number(form.stream_timeout) || 0,
    stream_idle_timeout: Number(form.stream_idle_timeout) || 0,
    max_idle_conns: Number(form.max_idle_conns) || 0,
    probe_interval: Number(form.probe_interval) || 0,
    probe_timeout: Number(form.probe_timeout) || 0,
    probe_fail_threshold: Number(form.probe_fail_threshold) || 0,
    probe_concurrency: Number(form.probe_concurrency) || 0,
    skip_unhealthy: form.skip_unhealthy,
    log_file: form.log_file,
    log_max_bytes: Number(form.log_max_bytes) || 0,
    log_retention_days: Number(form.log_retention_days) || 0,
    log_raw_tokens: form.log_raw_tokens,
    ring_size: Number(form.ring_size) || 0,
    retry_on_timeout: form.retry_on_timeout,
    rate_limit_per_minute: Number(form.rate_limit_per_minute) || 0,
    daily_per_token: Number(form.daily_per_token) || 0,
    admin_username: form.admin_username,
    admin_token: form.admin_token,
    login_enabled: !!form.login_enabled,
    admin_rate_per_minute: Number(form.admin_rate_per_minute) || 0,
    admin_allow_ips: form.admin_allow_ips
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
    metrics_token: form.metrics_token,
    route_strategy: form.route_strategy,
    alerts: {
      webhooks: form.alerts.webhooks,
      bark: form.alerts.bark,
      on_failover: form.alerts.on_failover,
      on_unhealthy: form.alerts.on_unhealthy,
      on_quota: form.alerts.on_quota,
      quota_warn_percent: Number(form.alerts.quota_warn_percent) || 0,
      min_alert_interval_seconds: Number(form.alerts.min_alert_interval_seconds) || 0,
    },
    aliases: aliasesObj,
    fallbacks: fallbacksObj,
    prices: pricesObj,
    alias_rules: rules,
  })

  // Leaving the password field empty keeps the current hash (server restores
  // masked values); entering one asks the server to set a new password.
  if (form.admin_password) base.admin_password = form.admin_password

  return base
}

async function save() {
  let payload: Raw
  try {
    payload = buildConfig()
  } catch (e) {
    toast.error((e as Error).message)
    return
  }
  if (!(await confirm('保存并热重载', '校验失败将保留当前配置。'))) return
  saving.value = true
  try {
    await request('/config', { method: 'POST', body: JSON.stringify(payload) })
    toast.success('已保存并热重载')
    await load()
  } catch (e) {
    handleError(e)
  } finally {
    saving.value = false
  }
}

async function exportConfig() {
  try {
    await download('/config/export', 'liapi-config.json')
  } catch (e) {
    handleError(e)
  }
}

async function importFile(file: File, format: 'liapi' | 'oneapi') {
  const content = await file.text()
  const msg =
    format === 'oneapi'
      ? '从 OneAPI 导入？仅替换 upstreams，其余配置保持不变。'
      : '导入并覆盖全部配置？当前配置将被替换（校验失败则不替换）。'
  if (!(await confirm('导入配置', msg, { danger: format !== 'oneapi' }))) return
  try {
    const path = format === 'oneapi' ? '/config/import?format=oneapi' : '/config/import'
    await request(path, { method: 'POST', body: content })
    toast.success(format === 'oneapi' ? 'OneAPI 上游已导入' : '导入成功')
    await load()
  } catch (e) {
    handleError(e)
  }
}

function onFile(e: Event, format: 'liapi' | 'oneapi') {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  if (f) importFile(f, format)
  input.value = ''
}

const newWebhook = ref('')
const newBark = ref('')
function addWebhook() {
  const v = newWebhook.value.trim()
  if (v) {
    form.alerts.webhooks.push(v)
    newWebhook.value = ''
  }
}
function addBark() {
  const v = newBark.value.trim()
  if (v) {
    form.alerts.bark.push(v)
    newBark.value = ''
  }
}
</script>

<template>
  <div class="space-y-4">
    <Card padding="none">
      <template #header>
        <div class="flex flex-wrap items-center gap-2 px-5 py-4">
          <h2 class="text-sm font-semibold">配置（可视化编辑 + 热重载）</h2>
          <div class="ml-auto flex flex-wrap items-center gap-2">
            <Segmented
              v-model="view"
              :options="[
                { label: '表单', value: 'form', icon: Eye },
                { label: 'JSON', value: 'json', icon: Code2 },
              ]"
              size="sm"
            />
            <Button variant="outline" size="sm" :icon="RefreshCw" :loading="loading" @click="load">
              重新加载
            </Button>
            <Button variant="primary" size="sm" :icon="Save" :loading="saving" @click="save">
              保存并热重载
            </Button>
            <Button variant="outline" size="sm" :icon="Download" @click="exportConfig">
              导出 JSON
            </Button>
            <label
              class="inline-flex cursor-pointer items-center gap-2 rounded-lg border border-border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-muted"
            >
              <Upload :size="15" /> 导入 JSON
              <input type="file" accept=".json,application/json" class="hidden" @change="onFile($event, 'liapi')" />
            </label>
            <label
              class="inline-flex cursor-pointer items-center gap-2 rounded-lg border border-border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-muted"
            >
              <FileJson :size="15" /> 导入 OneAPI
              <input type="file" accept=".json,application/json" class="hidden" @change="onFile($event, 'oneapi')" />
            </label>
          </div>
        </div>
      </template>

      <div class="space-y-4 px-5 pb-5">
        <Alert type="info" :show-icon="true">
          「重新加载」拉取的配置中密钥已脱敏，保存时脱敏值自动还原。
          表单未覆盖的字段（token_policies / token_quotas / upstreams / devices 等）
          保留原值：upstreams 与 devices 请到各自的「上游」「设备」页维护。
          「导出 JSON」含完整密钥，请妥善保管。
        </Alert>

        <!-- Visual form -->
        <Collapse v-if="view === 'form'" v-model="openPanels">
          <CollapseItem name="network" title="监听与转发">
            <div class="grid grid-cols-1 gap-x-6 gap-y-1 px-1 sm:grid-cols-2 lg:grid-cols-3">
              <Field label="addr（监听地址，重启生效）">
                <Input v-model="form.addr" placeholder="0.0.0.0 或 127.0.0.1" />
              </Field>
              <Field label="port（监听端口，重启生效）">
                <InputNumber v-model="form.port" :min="0" :max="65535" :step="1" controls placeholder="8787" />
                <span class="text-[11px] text-muted-foreground">0 = 默认 8787；修改后需重启 liapi</span>
              </Field>
              <Field label="body_limit_bytes（请求体上限）">
                <InputNumber v-model="form.body_limit_bytes" :min="0" :step="1048576" controls />
                <span class="text-[11px] text-muted-foreground">{{ prettyBytes(form.body_limit_bytes) }}</span>
              </Field>
              <Field label="max_idle_conns（连接池）">
                <InputNumber v-model="form.max_idle_conns" :min="0" controls />
              </Field>
              <Field label="timeout（非流式超时，秒；0=关闭）">
                <InputNumber v-model="form.timeout" :min="0" controls />
              </Field>
              <Field label="stream_timeout（流式整体超时，秒；0=不设）">
                <InputNumber v-model="form.stream_timeout" :min="0" controls />
              </Field>
              <Field label="stream_idle_timeout（流式空闲看门狗，秒）">
                <InputNumber v-model="form.stream_idle_timeout" :min="0" controls />
              </Field>
            </div>
          </CollapseItem>

          <CollapseItem name="routing" title="路由与重试">
            <div class="grid grid-cols-1 gap-x-6 gap-y-1 px-1 sm:grid-cols-2 lg:grid-cols-3">
              <Field label="route_strategy（路由策略）">
                <Segmented v-model="form.route_strategy" :options="routeOptions" size="sm" block />
              </Field>
              <Field label="retry_on_timeout（超时重试，防重复计费）">
                <div class="flex h-9 items-center">
                  <Switch v-model="form.retry_on_timeout" />
                </div>
              </Field>
              <Field label="skip_unhealthy（避开不健康上游）">
                <div class="flex h-9 items-center">
                  <Switch v-model="form.skip_unhealthy" />
                </div>
              </Field>
              <Field label="rate_limit_per_minute（全局限流，0=关）">
                <InputNumber v-model="form.rate_limit_per_minute" :min="0" controls />
              </Field>
              <Field label="daily_per_token（每日配额，0=关）">
                <InputNumber v-model="form.daily_per_token" :min="0" controls />
              </Field>
            </div>
          </CollapseItem>

          <CollapseItem name="health" title="健康检查">
            <div class="grid grid-cols-1 gap-x-6 gap-y-1 px-1 sm:grid-cols-2 lg:grid-cols-3">
              <Field label="probe_interval（间隔，秒）">
                <InputNumber v-model="form.probe_interval" :min="0" controls />
              </Field>
              <Field label="probe_timeout（超时，秒）">
                <InputNumber v-model="form.probe_timeout" :min="0" controls />
              </Field>
              <Field label="probe_fail_threshold（连续失败判定）">
                <InputNumber v-model="form.probe_fail_threshold" :min="0" controls />
              </Field>
              <Field label="probe_concurrency（探测并发）">
                <InputNumber v-model="form.probe_concurrency" :min="0" controls />
              </Field>
            </div>
          </CollapseItem>

          <CollapseItem name="logging" title="日志">
            <div class="grid grid-cols-1 gap-x-6 gap-y-1 px-1 sm:grid-cols-2 lg:grid-cols-3">
              <Field label="log_file（JSONL 路径）">
                <Input v-model="form.log_file" placeholder="relay.jsonl" />
              </Field>
              <Field label="log_max_bytes（轮转阈值）">
                <InputNumber v-model="form.log_max_bytes" :min="0" :step="1048576" controls />
                <span class="text-[11px] text-muted-foreground">{{ prettyBytes(form.log_max_bytes) }}</span>
              </Field>
              <Field label="log_retention_days（保留天数，0=永久）">
                <InputNumber v-model="form.log_retention_days" :min="0" controls />
              </Field>
              <Field label="ring_size（内存环形缓冲条数）">
                <InputNumber v-model="form.ring_size" :min="0" controls />
              </Field>
              <Field label="log_raw_tokens（记录明文 token，仅排查用）">
                <div class="flex h-9 items-center">
                  <Switch v-model="form.log_raw_tokens" />
                </div>
              </Field>
            </div>
          </CollapseItem>

          <CollapseItem name="admin" title="管理台与鉴权">
            <div class="grid grid-cols-1 gap-x-6 gap-y-1 px-1 sm:grid-cols-2 lg:grid-cols-3">
              <Field label="用户名/密码登录（login_enabled）">
                <div class="flex h-9 items-center gap-2">
                  <Switch v-model="form.login_enabled" />
                  <span class="text-[11px] text-muted-foreground">
                    {{ form.login_enabled ? '开启：用下方账号密码登录' : '关闭：仅 Admin Token 可进管理台' }}
                  </span>
                </div>
                <span class="text-[11px] text-muted-foreground">
                  开启需先设置密码；关闭后 Admin Token 仍然可用
                </span>
              </Field>
              <Field label="admin_username（登录用户名）">
                <Input v-model="form.admin_username" />
              </Field>
              <Field label="admin_password（留空=保持原密码）">
                <Input v-model="form.admin_password" type="password" placeholder="输入新密码以修改" />
              </Field>
              <Field label="admin_token（脱敏；原样保存则不变）">
                <Input v-model="form.admin_token" />
              </Field>
              <Field label="admin_rate_per_minute（-1=关闭，0=默认60）">
                <InputNumber v-model="form.admin_rate_per_minute" :min="-1" controls />
              </Field>
              <Field label="metrics_token（/metrics 鉴权；- = 免鉴权）">
                <Input v-model="form.metrics_token" placeholder="留空=用 admin_token" />
              </Field>
              <Field label="admin_allow_ips（IP/CIDR，逗号分隔）">
                <Input v-model="form.admin_allow_ips" placeholder="127.0.0.1, 10.0.0.0/8" />
              </Field>
            </div>
          </CollapseItem>

          <CollapseItem name="aliases" title="别名 / 降级链 / 价目">
            <div class="space-y-5 px-1">
              <div>
                <div class="mb-2 flex items-center">
                  <span class="text-xs font-medium">aliases（精确别名）</span>
                  <Button variant="outline" size="sm" :icon="Plus" class="ml-auto"
                    @click="aliases.push({ k: '', v: '' })">添加</Button>
                </div>
                <div v-for="(row, i) in aliases" :key="'a' + i" class="mb-2 flex items-center gap-2">
                  <Input v-model="row.k" placeholder="gpt-4o" />
                  <span class="text-muted-foreground">→</span>
                  <Input v-model="row.v" placeholder="openai/gpt-4o" />
                  <Button variant="ghost" size="sm" danger :icon="Trash2" @click="aliases.splice(i, 1)" />
                </div>
              </div>

              <div>
                <div class="mb-2 flex items-center">
                  <span class="text-xs font-medium">fallbacks（显式降级链，逗号分隔上游名）</span>
                  <Button variant="outline" size="sm" :icon="Plus" class="ml-auto"
                    @click="fallbacks.push({ model: '', chain: '' })">添加</Button>
                </div>
                <div v-for="(row, i) in fallbacks" :key="'f' + i" class="mb-2 flex items-center gap-2">
                  <Input v-model="row.model" placeholder="gpt-4o" />
                  <span class="text-muted-foreground">→</span>
                  <Input v-model="row.chain" placeholder="openai-main, openrouter-fallback" />
                  <Button variant="ghost" size="sm" danger :icon="Trash2" @click="fallbacks.splice(i, 1)" />
                </div>
              </div>

              <div>
                <div class="mb-2 flex items-center">
                  <span class="text-xs font-medium">prices（元 / 1M tokens）</span>
                  <Button variant="outline" size="sm" :icon="Plus" class="ml-auto"
                    @click="prices.push({ model: '', input: 0, output: 0 })">添加</Button>
                </div>
                <div v-for="(row, i) in prices" :key="'p' + i" class="mb-2 flex items-center gap-2">
                  <Input v-model="row.model" placeholder="gpt-4o" />
                  <InputNumber v-model="row.input" :min="0" :step="0.1" placeholder="input" controls />
                  <InputNumber v-model="row.output" :min="0" :step="0.1" placeholder="output" controls />
                  <Button variant="ghost" size="sm" danger :icon="Trash2" @click="prices.splice(i, 1)" />
                </div>
              </div>

              <div>
                <div class="mb-2 flex items-center">
                  <span class="text-xs font-medium">alias_rules（正则别名 + 参数覆写）</span>
                  <Button variant="outline" size="sm" :icon="Plus" class="ml-auto"
                    @click="aliasRules.push({ pattern: '', regex: false, model: '', params: '' })">添加</Button>
                </div>
                <div v-for="(row, i) in aliasRules" :key="'r' + i"
                  class="mb-2 grid grid-cols-1 items-center gap-2 sm:grid-cols-[1fr_auto_1fr_1fr_auto]">
                  <Input v-model="row.pattern" placeholder="^gpt-4o-mini$" />
                  <label class="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <Switch v-model="row.regex" size="sm" /> regex
                  </label>
                  <Input v-model="row.model" placeholder="openai/gpt-4o-mini" />
                  <Input v-model="row.params" placeholder='{"temperature":0.2}' />
                  <Button variant="ghost" size="sm" danger :icon="Trash2" @click="aliasRules.splice(i, 1)" />
                </div>
              </div>
            </div>
          </CollapseItem>

          <CollapseItem name="alerts" title="告警">
            <div class="space-y-4 px-1">
              <div class="grid grid-cols-1 gap-x-6 gap-y-1 sm:grid-cols-2 lg:grid-cols-4">
                <Field label="on_failover（故障转移告警）">
                  <div class="flex h-9 items-center"><Switch v-model="form.alerts.on_failover" /></div>
                </Field>
                <Field label="on_unhealthy（不健康告警）">
                  <div class="flex h-9 items-center"><Switch v-model="form.alerts.on_unhealthy" /></div>
                </Field>
                <Field label="on_quota（配额告警）">
                  <div class="flex h-9 items-center"><Switch v-model="form.alerts.on_quota" /></div>
                </Field>
                <Field label="quota_warn_percent（告警阈值 %）">
                  <InputNumber v-model="form.alerts.quota_warn_percent" :min="0" :max="100" controls />
                </Field>
                <Field label="min_alert_interval_seconds（去重窗口）">
                  <InputNumber v-model="form.alerts.min_alert_interval_seconds" :min="0" controls />
                </Field>
              </div>

              <div>
                <div class="mb-2 text-xs font-medium">webhooks（通用 JSON POST）</div>
                <div v-for="(w, i) in form.alerts.webhooks" :key="'w' + i" class="mb-2 flex items-center gap-2">
                  <Input :model-value="w" @update:model-value="(v) => (form.alerts.webhooks[i] = String(v))" />
                  <Button variant="ghost" size="sm" danger :icon="Trash2" @click="form.alerts.webhooks.splice(i, 1)" />
                </div>
                <div class="flex items-center gap-2">
                  <Input v-model="newWebhook" placeholder="https://hook.example.com/notify" @keyup.enter="addWebhook" />
                  <Button variant="outline" size="sm" :icon="Plus" @click="addWebhook">添加</Button>
                </div>
              </div>

              <div>
                <div class="mb-2 text-xs font-medium">bark（Bark 推送 URL）</div>
                <div v-for="(b, i) in form.alerts.bark" :key="'b' + i" class="mb-2 flex items-center gap-2">
                  <Input :model-value="b" @update:model-value="(v) => (form.alerts.bark[i] = String(v))" />
                  <Button variant="ghost" size="sm" danger :icon="Trash2" @click="form.alerts.bark.splice(i, 1)" />
                </div>
                <div class="flex items-center gap-2">
                  <Input v-model="newBark" placeholder="https://api.day.app/xxxx" @keyup.enter="addBark" />
                  <Button variant="outline" size="sm" :icon="Plus" @click="addBark">添加</Button>
                </div>
              </div>
            </div>
          </CollapseItem>
        </Collapse>

        <!-- Raw JSON -->
        <div v-else>
          <p class="mb-3 text-xs text-muted-foreground">
            直接编辑 JSON。保存时会先通过 JSON 解析与后端校验，失败则保留当前配置。
          </p>
          <Textarea
            v-model="text"
            :rows="24"
            resize="vertical"
            placeholder="{ }"
            class="font-mono text-xs"
          />
        </div>
      </div>
    </Card>
  </div>
</template>
