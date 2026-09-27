<script setup lang="ts">
import {
  Card,
  Button,
  Table,
  Chip,
  Input,
  Switch,
  Empty,
} from 'fuxsto-design'
import { Plus, Pencil, Trash2 } from 'lucide-vue-next'
import type { Upstream, HealthStatus } from '~/types/api'

const { request, setToken } = useApi()
const { toast, confirm, handleError } = useUi()

const list = ref<Upstream[]>([])
const health = ref<Record<string, HealthStatus>>({})
const loading = ref(true)
const saving = ref(false)
const showForm = ref(false)
const editing = ref<string | null>(null)

const form = reactive({
  name: '',
  base_url: '',
  api_key: '',
  models: '',
  health_path: '',
  retry: 0,
  priority: 1,
  weight: 1,
  disabled: false,
  inject_usage: false,
  group: '',
  model_map: '',
})

const columns = [
  { key: 'name', title: '名称' },
  { key: 'base_url', title: 'Base URL' },
  { key: 'models', title: '模型' },
  { key: 'priority', title: '优先级', width: 70 },
  { key: 'weight', title: '权重', width: 60 },
  { key: 'key', title: 'Key', width: 130 },
  { key: 'status', title: '状态', width: 90 },
  { key: 'actions', title: '', width: 120, align: 'right' as const },
]

async function load() {
  try {
    const [ups, h] = await Promise.all([
      request<Upstream[]>('/upstreams'),
      request<HealthStatus[]>('/health').catch(() => []),
    ])
    list.value = ups
    health.value = Object.fromEntries(h.map((x) => [x.name, x]))
  } catch (e) {
    handleError(e, setToken)
  } finally {
    loading.value = false
  }
}

onMounted(load)

function resetForm(u?: Upstream) {
  if (u) {
    editing.value = u.name
    Object.assign(form, {
      name: u.name,
      base_url: u.base_url || '',
      api_key: u.api_key || '',
      models: (u.models || []).join(','),
      health_path: u.health_path || '',
      retry: u.retry || 0,
      priority: u.priority || 1,
      weight: u.weight || 1,
      disabled: !!u.disabled,
      inject_usage: !!u.inject_usage,
      group: u.group || '',
      model_map: u.model_map ? JSON.stringify(u.model_map) : '',
    })
  } else {
    editing.value = null
    Object.assign(form, {
      name: '',
      base_url: '',
      api_key: '',
      models: '',
      health_path: '',
      retry: 0,
      priority: 1,
      weight: 1,
      disabled: false,
      inject_usage: false,
      group: '',
      model_map: '',
    })
  }
  showForm.value = true
}

async function save() {
  const models = form.models.split(',').map((s) => s.trim()).filter(Boolean)
  const obj: Record<string, unknown> = {
    name: form.name.trim(),
    base_url: form.base_url.trim(),
    models,
    health_path: form.health_path.trim(),
    retry: Number(form.retry) || 0,
    priority: Number(form.priority) || 0,
    weight: Math.max(1, Number(form.weight) || 1),
    disabled: form.disabled,
    inject_usage: form.inject_usage,
    group: form.group.trim(),
  }
  const mm = form.model_map.trim()
  if (mm) {
    try {
      obj.model_map = JSON.parse(mm)
    } catch {
      toast.error('model_map 不是合法 JSON')
      return
    }
  }
  if (form.api_key) obj.api_key = form.api_key

  saving.value = true
  try {
    const path = editing.value
      ? '/upstreams/' + encodeURIComponent(editing.value)
      : '/upstreams'
    await request(path, {
      method: editing.value ? 'PUT' : 'POST',
      body: JSON.stringify(obj),
    })
    toast.success('已保存')
    showForm.value = false
    await load()
  } catch (e) {
    handleError(e, setToken)
  } finally {
    saving.value = false
  }
}

async function remove(name: string) {
  if (!(await confirm('删除上游', `确定删除上游 ${name} ？`, { danger: true }))) return
  try {
    await request('/upstreams/' + encodeURIComponent(name), { method: 'DELETE' })
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
          <h2 class="text-sm font-semibold">上游配置</h2>
          <p class="text-xs text-muted-foreground">
            聚合多个上游，统一路由 / 重试 / 故障转移
          </p>
        </div>
        <Button variant="primary" size="sm" :icon="Plus" @click="resetForm()">
          新增上游
        </Button>
      </div>
    </template>

    <div class="px-5 pb-5">
      <Table
        v-if="list.length || loading"
        :columns="columns"
        :data="list"
        row-key="name"
        size="sm"
        hover
        :loading="loading"
        empty-text="还没有上游，点「新增上游」添加。"
      >
        <template #cell-name="{ row }">
          <span class="font-medium">{{ row.name }}</span>
        </template>
        <template #cell-base_url="{ row }">
          <span class="text-xs text-muted-foreground">{{ row.base_url }}</span>
        </template>
        <template #cell-models="{ row }">
          <div class="flex flex-wrap gap-1">
            <Chip
              v-for="m in row.models"
              :key="m"
              size="sm"
              variant="secondary"
            >
              {{ m }}
            </Chip>
          </div>
        </template>
        <template #cell-priority="{ row }">{{ row.priority }}</template>
        <template #cell-weight="{ row }">{{ row.weight }}</template>
        <template #cell-key="{ row }">
          <span v-if="row.has_api_key" class="text-xs text-muted-foreground">
            {{ row.api_key }}
          </span>
          <Chip v-else size="sm" variant="outline" class="text-warning">无 key</Chip>
        </template>
        <template #cell-status="{ row }">
          <Chip v-if="row.disabled" size="sm" variant="outline">disabled</Chip>
          <Chip
            v-else-if="health[row.name]"
            size="sm"
            :variant="health[row.name]?.ok ? 'primary' : 'secondary'"
          >
            {{ health[row.name]?.ok ? '正常' : '异常' }}
          </Chip>
          <span v-else class="text-xs text-muted-foreground">—</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex justify-end gap-1">
            <Button
              variant="ghost"
              size="sm"
              :icon="Pencil"
              @click="resetForm(row)"
            />
            <Button
              variant="ghost"
              size="sm"
              danger
              :icon="Trash2"
              @click="remove(row.name)"
            />
          </div>
        </template>
      </Table>
      <Empty v-else title="还没有上游" description="点右上角「新增上游」添加第一个上游。" />
    </div>

    <DialogPanel
      v-model:open="showForm"
      :title="editing ? '编辑上游' : '新增上游'"
      size="lg"
    >
      <div class="grid grid-cols-1 gap-x-6 gap-y-1 sm:grid-cols-2">
        <Field label="name *">
          <Input v-model="form.name" placeholder="openai-main" />
        </Field>
        <Field label="base_url *（带版本前缀，无尾斜杠）">
          <Input v-model="form.base_url" placeholder="https://api.openai.com/v1" />
        </Field>
        <Field label="api_key（留空=保留原值）">
          <Input v-model="form.api_key" type="password" placeholder="sk-..." />
        </Field>
        <Field label="models *（逗号分隔，* 兜底）">
          <Input v-model="form.models" placeholder="gpt-4o,gpt-4o-mini" />
        </Field>
        <Field label="health_path">
          <Input v-model="form.health_path" placeholder="/v1/models" />
        </Field>
        <Field label="retry（0-10）">
          <Input v-model="form.retry" type="number" />
        </Field>
        <Field label="priority（越小越优先）">
          <Input v-model="form.priority" type="number" />
        </Field>
        <Field label="weight">
          <Input v-model="form.weight" type="number" />
        </Field>
        <Field label="group（配合 X-Route-Group 路由）">
          <Input v-model="form.group" placeholder="如 free / pro" />
        </Field>
        <Field label="model_map（JSON：公开名→上游真实名）">
          <Input v-model="form.model_map" placeholder='{"gpt-4o":"gpt-4o-2024-11-20"}' />
        </Field>
        <div class="flex items-center gap-6 pt-2 sm:col-span-2">
          <label class="flex items-center gap-2 text-sm">
            <Switch v-model="form.disabled" /> <span>disabled</span>
          </label>
          <label class="flex items-center gap-2 text-sm">
            <Switch v-model="form.inject_usage" />
            <span>流式注入 include_usage</span>
          </label>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <Button variant="outline" size="sm" @click="showForm = false">取消</Button>
          <Button variant="primary" size="sm" :loading="saving" @click="save">
            保存
          </Button>
        </div>
      </template>
    </DialogPanel>
  </Card>
</template>
