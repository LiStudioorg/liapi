<script setup lang="ts">
import { Card, Button, Textarea } from 'fuxsto-design'
import { RefreshCw, Save, Download, Upload, FileJson } from 'lucide-vue-next'

const { request, download } = useApi()
const { toast, confirm, handleError } = useUi()

const text = ref('')
const saving = ref(false)
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    const cfg = await request<Record<string, unknown>>('/config')
    text.value = JSON.stringify(cfg, null, 2)
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function save() {
  try {
    JSON.parse(text.value)
  } catch (e) {
    toast.error('不是合法 JSON: ' + (e as Error).message)
    return
  }
  if (!(await confirm('保存并热重载', '校验失败将保留当前配置。'))) return
  saving.value = true
  try {
    await request('/config', { method: 'POST', body: text.value })
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
</script>

<template>
  <Card padding="none">
    <template #header>
      <div class="flex flex-wrap items-center gap-2 px-5 py-4">
        <h2 class="text-sm font-semibold">配置（在线编辑 + 热重载）</h2>
        <div class="ml-auto flex flex-wrap items-center gap-2">
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

    <div class="px-5 pb-5">
      <p class="mb-3 text-xs text-muted-foreground">
        「重新加载」拉取的配置中密钥已脱敏，保存时脱敏值会自动还原为原密钥。
        「导出 JSON」包含完整密钥（含 admin_token / api_key），请妥善保管。
        管理台登录账号为 admin_username + 密码（admin_password_hash 存 PBKDF2 哈希，不可回读）。
        OneAPI 导入仅替换 upstreams 列表，其余配置保持不变。
      </p>
      <Textarea
        v-model="text"
        :rows="22"
        resize="vertical"
        placeholder="{ }"
        class="font-mono text-xs"
      />
    </div>
  </Card>
</template>
