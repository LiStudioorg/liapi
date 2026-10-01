<script setup lang="ts">
import { Card, Button, Table, Input, Select, Chip } from 'fuxsto-design'
import { RefreshCw } from 'lucide-vue-next'
import type { LogEntry } from '~/types/api'
import { fmtNum, fmtTime, statusTone } from '~/utils/format'

const { request } = useApi()
const { handleError } = useUi()

const list = ref<LogEntry[]>([])
const loading = ref(true)
const q = ref('')
const status = ref('')

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '200', value: '200' },
  { label: '400', value: '400' },
  { label: '401', value: '401' },
  { label: '429', value: '429' },
  { label: '500', value: '500' },
  { label: '502', value: '502' },
  { label: '504', value: '504' },
]

const columns = [
  { key: 'time', title: '时间', width: 90 },
  { key: 'token', title: 'Token', width: 110 },
  { key: 'model', title: '模型' },
  { key: 'upstream', title: '上游', width: 120 },
  { key: 'status', title: '状态', width: 80 },
  { key: 'stream', title: '流', width: 50 },
  { key: 'latency_ms', title: '延迟', width: 80 },
  { key: 'io', title: 'in/out', width: 110 },
  { key: 'error', title: '错误' },
]

let timer: ReturnType<typeof setTimeout> | undefined

async function load() {
  try {
    let url = '/logs?n=200'
    if (q.value.trim()) url += '&q=' + encodeURIComponent(q.value.trim())
    if (status.value) url += '&status=' + encodeURIComponent(status.value)
    list.value = await request<LogEntry[]>(url)
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}

function debounced() {
  clearTimeout(timer)
  timer = setTimeout(load, 300)
}

onMounted(load)
</script>

<template>
  <Card padding="none">
    <template #header>
      <div class="flex flex-wrap items-center gap-3 px-5 py-4">
        <h2 class="text-sm font-semibold">最近日志</h2>
        <div class="ml-auto flex flex-wrap items-center gap-2">
          <div class="w-64">
            <Input
              v-model="q"
              size="sm"
              placeholder="搜索 model / 上游 / 错误 / token…"
              @input="debounced"
            />
          </div>
          <div class="w-32">
            <Select
              v-model="status"
              :options="statusOptions"
              size="sm"
              @change="load"
            />
          </div>
          <Button variant="outline" size="sm" :icon="RefreshCw" @click="load">
            刷新
          </Button>
        </div>
      </div>
    </template>

    <div class="px-5 pb-5">
      <Table
        :columns="columns"
        :data="list"
        size="sm"
        hover
        :loading="loading"
        empty-text="暂无日志。"
      >
        <template #cell-time="{ row }">{{ fmtTime(row.time) }}</template>
        <template #cell-token="{ row }">
          <span class="text-xs text-muted-foreground">{{ row.token }}</span>
        </template>
        <template #cell-upstream="{ row }">{{ row.upstream || '-' }}</template>
        <template #cell-status="{ row }">
          <Chip
            size="sm"
            :variant="statusTone(row.status) === 'success' ? 'primary' : 'outline'"
          >
            {{ row.status }}
          </Chip>
        </template>
        <template #cell-stream="{ row }">{{ row.stream ? 'Y' : '' }}</template>
        <template #cell-latency_ms="{ row }">{{ row.latency_ms }}ms</template>
        <template #cell-io="{ row }">
          {{ fmtNum(row.in_tokens) }}/{{ fmtNum(row.out_tokens) }}
        </template>
        <template #cell-error="{ row }">
          <span class="text-xs text-muted-foreground">{{ row.error }}</span>
        </template>
      </Table>
    </div>
  </Card>
</template>
