<script setup lang="ts">
import { Card, Statistic, Skeleton } from 'fuxsto-design'
import type { Overview } from '~/types/api'
import { fmtNum, fmtCost } from '~/utils/format'

const { request, setToken } = useApi()
const { handleError } = useUi()

const data = ref<Overview | null>(null)
const loading = ref(true)

async function load() {
  try {
    data.value = await request<Overview>('/overview')
  } catch (e) {
    handleError(e, setToken)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
  const t = setInterval(load, 4000)
  onBeforeUnmount(() => clearInterval(t))
})

const cards = computed(() => {
  const d = data.value
  if (!d) return []
  return [
    { title: '请求数', value: fmtNum(d.requests), suffix: '' },
    { title: '成功率', value: (d.success_rate || 0).toFixed(1), suffix: '%' },
    { title: '平均延迟', value: d.avg_latency_ms || 0, suffix: 'ms' },
    { title: 'in_token', value: fmtNum(d.in_tokens) },
    { title: 'out_token', value: fmtNum(d.out_tokens) },
    { title: '费用(元)', value: fmtCost(d.cost) },
    { title: '今日费用', value: fmtCost(d.cost_today) },
    { title: '客户端错误', value: fmtNum(d.client_errors) },
    { title: '上游错误', value: fmtNum(d.upstream_errors) },
    { title: '故障转移', value: fmtNum(d.failovers) },
    { title: '多模态部件', value: fmtNum(d.multimodal) },
    { title: '上游数', value: fmtNum(d.upstreams) },
    { title: '客户端 Token', value: fmtNum(d.client_tokens) },
    { title: '设备数', value: fmtNum(d.devices) },
    { title: '模型数', value: fmtNum(d.models) },
    { title: '丢弃日志', value: fmtNum(d.dropped_logs) },
  ]
})
</script>

<template>
  <div class="space-y-4">
    <div v-if="loading && !data" class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
      <Skeleton v-for="i in 8" :key="i" class="h-[88px] rounded-xl" />
    </div>

    <div
      v-else
      class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5"
    >
      <Card
        v-for="(c, i) in cards"
        :key="c.title"
        padding="md"
        hover-shadow
        class="transition-transform duration-200 hover:-translate-y-0.5"
      >
        <Statistic
          :title="c.title"
          :value="c.value"
          :suffix="c.suffix"
          size="md"
          animate
          :delay="i * 25"
        />
      </Card>
    </div>
  </div>
</template>
