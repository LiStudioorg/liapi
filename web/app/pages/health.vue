<script setup lang="ts">
import { Card, Button, Table, Chip } from 'fuxsto-design'
import { Activity, RefreshCw, ShieldCheck } from 'lucide-vue-next'
import type { HealthStatus, ProbeEvent, AuditEvent } from '~/types/api'
import { fmtTime } from '~/utils/format'

const { request } = useApi()
const { toast, handleError } = useUi()

const health = ref<HealthStatus[]>([])
const history = ref<ProbeEvent[]>([])
const audit = ref<AuditEvent[]>([])
const loading = ref(true)
const probing = ref(false)

async function loadAll() {
  try {
    const [h, hi, a] = await Promise.all([
      request<HealthStatus[]>('/health'),
      request<ProbeEvent[]>('/health/history?n=50'),
      request<AuditEvent[]>('/audit?n=50'),
    ])
    health.value = h
    history.value = hi
    audit.value = a
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}
onMounted(loadAll)

async function probe() {
  probing.value = true
  try {
    await request('/health/probe', { method: 'POST', body: '{}' })
    toast.success('探测完成')
    await loadAll()
  } catch (e) {
    handleError(e)
  } finally {
    probing.value = false
  }
}

const healthCols = [
  { key: 'name', title: '上游' },
  { key: 'ok', title: '状态', width: 90 },
  { key: 'latency_ms', title: '延迟', width: 90 },
  { key: 'consecutive_failures', title: '连续失败', width: 90 },
  { key: 'last_check', title: '最后检查', width: 180 },
  { key: 'error', title: '错误' },
]

const histCols = [
  { key: 'time', title: '时间', width: 90 },
  { key: 'name', title: '上游' },
  { key: 'ok', title: '状态', width: 80 },
  { key: 'latency_ms', title: '延迟', width: 80 },
  { key: 'source', title: '来源', width: 80 },
  { key: 'error', title: '错误' },
]

const auditCols = [
  { key: 'time', title: '时间', width: 90 },
  { key: 'ip', title: 'IP', width: 140 },
  { key: 'method', title: '方法', width: 80 },
  { key: 'path', title: '路径' },
  { key: 'ok', title: '结果', width: 80 },
  { key: 'reason', title: '原因' },
]
</script>

<template>
  <div class="space-y-4">
    <Card padding="none">
      <template #header>
        <div class="flex items-center justify-between gap-3 px-5 py-4">
          <div class="flex items-center gap-2">
            <Activity :size="16" class="text-muted-foreground" />
            <h2 class="text-sm font-semibold">上游健康状态</h2>
          </div>
          <Button
            variant="primary"
            size="sm"
            :icon="RefreshCw"
            :loading="probing"
            @click="probe"
          >
            立即探测
          </Button>
        </div>
      </template>
      <div class="px-5 pb-5">
        <Table
          :columns="healthCols"
          :data="health"
          size="sm"
          hover
          :loading="loading"
          empty-text="没有启用的上游。"
        >
          <template #cell-ok="{ row }">
            <Chip size="sm" :variant="row.ok ? 'primary' : 'outline'">
              {{ row.ok ? '正常' : '异常' }}
            </Chip>
          </template>
          <template #cell-latency_ms="{ row }">{{ row.latency_ms }}ms</template>
          <template #cell-error="{ row }">
            <span class="text-xs text-muted-foreground">{{ row.error }}</span>
          </template>
        </Table>
      </div>
    </Card>

    <Card padding="none">
      <template #header>
        <div class="flex items-center justify-between gap-3 px-5 py-4">
          <h2 class="text-sm font-semibold">探测历史</h2>
          <Button variant="outline" size="sm" :icon="RefreshCw" @click="loadAll">
            刷新
          </Button>
        </div>
      </template>
      <div class="px-5 pb-5">
        <Table
          :columns="histCols"
          :data="history"
          size="sm"
          hover
          :loading="loading"
          empty-text="暂无探测记录。"
        >
          <template #cell-time="{ row }">{{ fmtTime(row.time) }}</template>
          <template #cell-ok="{ row }">
            <Chip size="sm" :variant="row.ok ? 'primary' : 'outline'">
              {{ row.ok ? '正常' : '异常' }}
            </Chip>
          </template>
          <template #cell-latency_ms="{ row }">{{ row.latency_ms }}ms</template>
          <template #cell-source="{ row }">
            <Chip size="sm" :variant="row.manual ? 'secondary' : 'outline'">
              {{ row.manual ? '手动' : '定时' }}
            </Chip>
          </template>
          <template #cell-error="{ row }">
            <span class="text-xs text-muted-foreground">{{ row.error }}</span>
          </template>
        </Table>
      </div>
    </Card>

    <Card padding="none">
      <template #header>
        <div class="flex items-center justify-between gap-3 px-5 py-4">
          <div class="flex items-center gap-2">
            <ShieldCheck :size="16" class="text-muted-foreground" />
            <h2 class="text-sm font-semibold">管理端审计日志</h2>
          </div>
          <Button variant="outline" size="sm" :icon="RefreshCw" @click="loadAll">
            刷新
          </Button>
        </div>
      </template>
      <div class="px-5 pb-5">
        <Table
          :columns="auditCols"
          :data="audit"
          size="sm"
          hover
          :loading="loading"
          empty-text="暂无审计记录。"
        >
          <template #cell-time="{ row }">{{ fmtTime(row.time) }}</template>
          <template #cell-ok="{ row }">
            <Chip size="sm" :variant="row.ok ? 'primary' : 'outline'">
              {{ row.ok ? '成功' : '拒绝' }}
            </Chip>
          </template>
          <template #cell-reason="{ row }">
            <span class="text-xs text-muted-foreground">{{ row.reason }}</span>
          </template>
        </Table>
      </div>
    </Card>
  </div>
</template>
