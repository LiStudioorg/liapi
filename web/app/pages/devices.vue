<script setup lang="ts">
import { Card, Button, Table, Chip, Input, Switch, Alert, Empty } from 'fuxsto-design'
import { Plus, RefreshCw, Trash2 } from 'lucide-vue-next'
import type { Device, DeviceCreateResult } from '~/types/api'

const { request, setToken } = useApi()
const { toast, confirm, handleError } = useUi()

const list = ref<Device[]>([])
const loading = ref(true)
const saving = ref(false)
const showForm = ref(false)
const issuedToken = ref('')
const issuedTitle = ref('')

const form = reactive({
  id: '',
  name: '',
  note: '',
  rpm: 0,
  daily: 0,
  disabled: false,
})

const columns = [
  { key: 'id', title: 'ID' },
  { key: 'name', title: '名称' },
  { key: 'rpm', title: 'RPM', width: 80 },
  { key: 'daily', title: '每日', width: 80 },
  { key: 'token_hint', title: 'Token', width: 130 },
  { key: 'status', title: '状态', width: 90 },
  { key: 'actions', title: '', width: 110, align: 'right' as const },
]

async function load() {
  try {
    list.value = await request<Device[]>('/devices')
  } catch (e) {
    handleError(e, setToken)
  } finally {
    loading.value = false
  }
}
onMounted(load)

function resetForm() {
  Object.assign(form, { id: '', name: '', note: '', rpm: 0, daily: 0, disabled: false })
  showForm.value = true
}

async function create() {
  saving.value = true
  try {
    const d = await request<DeviceCreateResult>('/devices', {
      method: 'POST',
      body: JSON.stringify({
        id: form.id.trim(),
        name: form.name.trim(),
        note: form.note.trim(),
        rpm: Number(form.rpm) || 0,
        daily: Number(form.daily) || 0,
        disabled: form.disabled,
      }),
    })
    showForm.value = false
    if (d.token) {
      issuedTitle.value = '设备已创建'
      issuedToken.value = d.token
    }
    toast.success('设备已创建')
    await load()
  } catch (e) {
    handleError(e, setToken)
  } finally {
    saving.value = false
  }
}

async function rotate(id: string) {
  if (
    !(await confirm(
      '轮换 Token',
      `轮换 ${id} 的 token？旧 token 立即失效。`,
      { danger: true },
    ))
  )
    return
  try {
    const d = await request<DeviceCreateResult>(
      '/devices/' + encodeURIComponent(id) + '/rotate',
      { method: 'POST', body: '{}' },
    )
    if (d.token) {
      issuedTitle.value = '已轮换'
      issuedToken.value = d.token
    }
    toast.success('已轮换')
    await load()
  } catch (e) {
    handleError(e, setToken)
  }
}

async function remove(id: string) {
  if (!(await confirm('删除设备', `确定删除设备 ${id}？`, { danger: true }))) return
  try {
    await request('/devices/' + encodeURIComponent(id), { method: 'DELETE' })
    toast.success('已删除')
    await load()
  } catch (e) {
    handleError(e, setToken)
  }
}
</script>

<template>
  <Card padding="none">
    <template #header>
      <div class="flex items-center justify-between gap-3 px-5 py-4">
        <div>
          <h2 class="text-sm font-semibold">设备</h2>
          <p class="text-xs text-muted-foreground">
            Token 仅存 SHA-256 哈希，明文只显示一次
          </p>
        </div>
        <Button variant="primary" size="sm" :icon="Plus" @click="resetForm">
          新建设备
        </Button>
      </div>
    </template>

    <div class="space-y-4 px-5 pb-5">
      <Alert
        v-if="issuedToken"
        type="warning"
        title="此 token 只显示一次，请立即保存"
        closable
        @close="issuedToken = ''"
      >
        <code class="mt-1 block break-all text-xs">{{ issuedToken }}</code>
      </Alert>

      <Table
        v-if="list.length || loading"
        :columns="columns"
        :data="list"
        row-key="id"
        size="sm"
        hover
        :loading="loading"
        empty-text="还没有设备，点「新建设备」签发 token。"
      >
        <template #cell-id="{ row }"><code class="text-xs">{{ row.id }}</code></template>
        <template #cell-name="{ row }">{{ row.name || '-' }}</template>
        <template #cell-rpm="{ row }">{{ row.rpm > 0 ? row.rpm : '全局' }}</template>
        <template #cell-daily="{ row }">
          {{ row.daily > 0 ? row.daily : '全局' }}
        </template>
        <template #cell-token_hint="{ row }">
          <span class="text-xs text-muted-foreground">{{ row.token_hint }}</span>
        </template>
        <template #cell-status="{ row }">
          <Chip v-if="row.disabled" size="sm" variant="outline">disabled</Chip>
          <Chip v-else size="sm" variant="primary">启用</Chip>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex justify-end gap-1">
            <Button
              variant="ghost"
              size="sm"
              :icon="RefreshCw"
              @click="rotate(row.id)"
            />
            <Button
              variant="ghost"
              size="sm"
              danger
              :icon="Trash2"
              @click="remove(row.id)"
            />
          </div>
        </template>
      </Table>
      <Empty v-else title="还没有设备" description="点「新建设备」签发 token。" />
    </div>

    <DialogPanel v-model:open="showForm" title="新建设备" size="lg">
      <div class="grid grid-cols-1 gap-x-6 gap-y-1 sm:grid-cols-2">
        <Field label="id（留空自动生成）">
          <Input v-model="form.id" placeholder="phone-01" />
        </Field>
        <Field label="名称">
          <Input v-model="form.name" placeholder="我的手机" />
        </Field>
        <Field label="备注">
          <Input v-model="form.note" />
        </Field>
        <Field label="RPM（0=用全局）">
          <Input v-model="form.rpm" type="number" />
        </Field>
        <Field label="每日上限（0=用全局）">
          <Input v-model="form.daily" type="number" />
        </Field>
        <div class="flex items-center pt-6">
          <label class="flex items-center gap-2 text-sm">
            <Switch v-model="form.disabled" /> <span>disabled</span>
          </label>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <Button variant="outline" size="sm" @click="showForm = false">取消</Button>
          <Button variant="primary" size="sm" :loading="saving" @click="create">
            创建
          </Button>
        </div>
      </template>
    </DialogPanel>
  </Card>
</template>
