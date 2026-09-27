<script setup lang="ts">
import { Card, Button, Input, Textarea, Switch } from 'fuxsto-design'
import { Send } from 'lucide-vue-next'
import type { Config, TestResult } from '~/types/api'

const { request, setToken } = useApi()
const { handleError } = useUi()

const model = ref('')
const messages = ref('[{"role":"user","content":"你好，用一句话介绍自己。"}]')
const stream = ref(false)
const sending = ref(false)
const output = ref('')
const hasOutput = ref(false)
const models = ref<string[]>([])
const modelListId = 'liapi-model-list'

onMounted(async () => {
  try {
    const cfg = await request<Config>('/config')
    const names = new Set<string>()
    for (const u of cfg.upstreams || []) {
      if (u.disabled) continue
      for (const m of u.models || []) if (m !== '*') names.add(m)
    }
    for (const m of Object.keys(cfg.aliases || {})) names.add(m)
    models.value = [...names].sort()
  } catch {
    /* non-fatal */
  }
})

async function send() {
  let parsed: unknown
  try {
    parsed = JSON.parse(messages.value)
  } catch (e) {
    hasOutput.value = true
    output.value = 'messages 不是合法 JSON: ' + (e as Error).message
    return
  }
  sending.value = true
  hasOutput.value = true
  output.value = '请求中...'
  try {
    const d = await request<TestResult>('/test', {
      method: 'POST',
      body: JSON.stringify({ model: model.value.trim(), messages: parsed, stream: stream.value }),
    })
    output.value = JSON.stringify(d, null, 2)
  } catch (e) {
    const err = e as Error
    output.value = '错误: ' + err.message
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <Card padding="none">
    <template #header>
      <div class="px-5 py-4">
        <h2 class="text-sm font-semibold">调试：发送一条请求</h2>
        <p class="text-xs text-muted-foreground">
          直接走完整转发链路（别名 / 路由 / 故障转移），用于连通性排查
        </p>
      </div>
    </template>

    <div class="space-y-1 px-5 pb-5">
      <Field label="model">
        <Input v-model="model" :list="modelListId" placeholder="gpt-4o" />
        <datalist :id="modelListId">
          <option v-for="m in models" :key="m" :value="m" />
        </datalist>
      </Field>

      <Field label="messages (JSON 数组)">
        <Textarea v-model="messages" :rows="5" class="font-mono text-xs" />
      </Field>

      <div class="flex items-center gap-4 pt-3">
        <label class="flex items-center gap-2 text-sm">
          <Switch v-model="stream" /> <span>stream</span>
        </label>
        <div class="ml-auto">
          <Button variant="primary" size="md" :icon="Send" :loading="sending" @click="send">
            发送
          </Button>
        </div>
      </div>

      <Transition name="fade">
        <pre
          v-if="hasOutput"
          class="mt-3 max-h-[420px] overflow-auto rounded-lg border border-border bg-muted/40 p-3 text-xs leading-relaxed break-all whitespace-pre-wrap"
        >{{ output }}</pre>
      </Transition>
    </div>
  </Card>
</template>
