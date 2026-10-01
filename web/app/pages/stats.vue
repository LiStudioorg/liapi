<script setup lang="ts">
import { Card, Button, Table, Input, Chip, Select } from 'fuxsto-design'
import { Download, RefreshCw } from 'lucide-vue-next'
import type { StatsSnapshot, StatRow } from '~/types/api'
import { fmtNum, fmtCost } from '~/utils/format'

const { request, download } = useApi()
const { handleError, toast } = useUi()

const groupBy = ref<'model' | 'device' | 'day'>('model')
const from = ref('')
const to = ref('')
const snap = ref<StatsSnapshot | null>(null)
const top = ref<StatRow[]>([])
const loading = ref(true)

const groupOptions = [
  { label: '按模型', value: 'model' },
  { label: '按设备', value: 'device' },
  { label: '按天', value: 'day' },
]

const columns = [
  { key: 'key', title: 'Key' },
  { key: 'requests', title: '请求', width: 80 },
  { key: 'success', title: '成功', width: 80 },
  { key: 'errors', title: '错误', width: 70 },
  { key: 'io', title: 'in/out', width: 130 },
  { key: 'cost', title: '费用', width: 90 },
  { key: 'avg_latency_ms', title: 'avg', width: 70 },
  { key: 'p50_ms', title: 'P50', width: 60 },
  { key: 'p90_ms', title: 'P90', width: 60 },
  { key: 'p99_ms', title: 'P99', width: 60 },
]

async function load() {
  loading.value = true
  try {
    let qs = 'group_by=' + encodeURIComponent(groupBy.value)
    if (from.value) qs += '&from=' + from.value
    if (to.value) qs += '&to=' + to.value
    const [s, t] = await Promise.all([
      request<StatsSnapshot>('/stats?' + qs),
      request<StatRow[]>('/stats/devices?n=10').catch(() => []),
    ])
    snap.value = s
    top.value = t
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const total = computed(() => snap.value?.total)
const errorCodes = computed(() =>
  Object.entries(snap.value?.error_codes || {}).sort((a, b) =>
    a[0].localeCompare(b[0]),
  ),
)

function exportQs(fmt: string) {
  let qs = 'format=' + fmt + '&group_by=' + encodeURIComponent(groupBy.value)
  if (from.value) qs += '&from=' + from.value
  if (to.value) qs += '&to=' + to.value
  return qs
}

async function exportStats(fmt: 'csv' | 'json') {
  try {
    await download(
      '/stats/export?' + exportQs(fmt),
      fmt === 'csv' ? 'liapi-stats.csv' : 'liapi-stats.json',
    )
  } catch (e) {
    handleError(e)
  }
}
</script>

<template>
  <div class="space-y-4">
    <Card padding="none">
      <template #header>
        <div class="flex flex-wrap items-center gap-3 px-5 py-4">
          <h2 class="text-sm font-semibold">用量统计</h2>
          <div class="ml-auto flex flex-wrap items-center gap-2">
            <div class="w-32">
              <Select v-model="groupBy" :options="groupOptions" size="sm" @change="load" />
            </div>
            <input
              v-model="from"
              type="date"
              class="rounded-lg border border-border bg-background px-3 py-1.5 text-sm outline-none focus:border-ring"
            />
            <input
              v-model="to"
              type="date"
              class="rounded-lg border border-border bg-background px-3 py-1.5 text-sm outline-none focus:border-ring"
            />
            <Button variant="outline" size="sm" :icon="RefreshCw" @click="load">
              查询
            </Button>
            <Button variant="outline" size="sm" :icon="Download" @click="exportStats('csv')">
              CSV
            </Button>
            <Button variant="outline" size="sm" :icon="Download" @click="exportStats('json')">
              JSON
            </Button>
          </div>
        </div>
      </template>

      <div class="space-y-5 px-5 pb-5">
        <div v-if="total" class="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-7">
          <StatTile label="总请求" :value="fmtNum(total.requests)" />
          <StatTile label="成功" :value="fmtNum(total.success)" />
          <StatTile label="错误" :value="fmtNum(total.errors)" />
          <StatTile label="费用(元)" :value="fmtCost(total.cost)" />
          <StatTile label="P50" :value="total.p50_ms + 'ms'" />
          <StatTile label="P90" :value="total.p90_ms + 'ms'" />
          <StatTile label="P99" :value="total.p99_ms + 'ms'" />
        </div>

        <Table
          :columns="columns"
          :data="snap?.rows || []"
          size="sm"
          hover
          :loading="loading"
          empty-text="所选区间暂无数据。"
        >
          <template #cell-key="{ row }"><span class="font-medium">{{ row.key }}</span></template>
          <template #cell-io="{ row }">
            {{ fmtNum(row.in_tokens) }}/{{ fmtNum(row.out_tokens) }}
          </template>
          <template #cell-cost="{ row }">{{ fmtCost(row.cost) }}</template>
        </Table>

        <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <div>
            <h3 class="mb-2 text-sm font-semibold">设备排行</h3>
            <Table
              :columns="[
                { key: 'key', title: '设备/Token' },
                { key: 'requests', title: '请求', width: 80 },
                { key: 'io', title: 'in/out', width: 130 },
                { key: 'cost', title: '费用', width: 90 },
              ]"
              :data="top"
              size="sm"
              hover
              empty-text="暂无数据。"
            >
              <template #cell-io="{ row }">
                {{ fmtNum(row.in_tokens) }}/{{ fmtNum(row.out_tokens) }}
              </template>
              <template #cell-cost="{ row }">{{ fmtCost(row.cost) }}</template>
            </Table>
          </div>
          <div>
            <h3 class="mb-2 text-sm font-semibold">错误码分布</h3>
            <div v-if="errorCodes.length" class="flex flex-wrap gap-2">
              <Chip v-for="[code, n] in errorCodes" :key="code" variant="secondary">
                HTTP {{ code }} · {{ n }}
              </Chip>
            </div>
            <p v-else class="text-sm text-muted-foreground">无错误</p>
          </div>
        </div>
      </div>
    </Card>
  </div>
</template>
