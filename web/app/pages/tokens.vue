<script setup lang="ts">
import { Card, Button, Table, Input, Empty } from 'fuxsto-design'
import { Plus, Trash2 } from 'lucide-vue-next'
import type { ClientToken } from '~/types/api'

const { request, setToken } = useApi()
const { toast, confirm, handleError } = useUi()

const list = ref<ClientToken[]>([])
const loading = ref(true)
const newToken = ref('')

const columns = [
  { key: 'masked', title: 'Token' },
  { key: 'actions', title: '', width: 80, align: 'right' as const },
]

async function load() {
  try {
    list.value = await request<ClientToken[]>('/tokens')
  } catch (e) {
    handleError(e, setToken)
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function add() {
  const v = newToken.value.trim()
  if (!v) return toast.warning('请输入 token')
  try {
    await request('/tokens', { method: 'POST', body: JSON.stringify({ token: v }) })
    newToken.value = ''
    toast.success('已添加')
    await load()
  } catch (e) {
    handleError(e, setToken)
  }
}

async function remove(full: string) {
  if (!(await confirm('删除令牌', '确定删除该 token？', { danger: true }))) return
  try {
    await request('/tokens/delete', {
      method: 'POST',
      body: JSON.stringify({ token: full }),
    })
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
      <div class="px-5 py-4">
        <h2 class="text-sm font-semibold">客户端令牌</h2>
        <p class="text-xs text-muted-foreground">
          明文列表，兼容旧版；新接入推荐用「设备」
        </p>
      </div>
    </template>

    <div class="space-y-4 px-5 pb-5">
      <div class="flex max-w-md gap-2">
        <Input
          v-model="newToken"
          placeholder="输入新 token (如 sk-client-xxxx)"
          @keydown.enter="add"
        />
        <Button variant="primary" size="md" :icon="Plus" @click="add">添加</Button>
      </div>

      <Table
        v-if="list.length || loading"
        :columns="columns"
        :data="list"
        size="sm"
        hover
        :loading="loading"
        empty-text="还没有客户端令牌。"
      >
        <template #cell-masked="{ row }">
          <code class="text-xs">{{ row.masked }}</code>
        </template>
        <template #cell-actions="{ row }">
          <Button
            variant="ghost"
            size="sm"
            danger
            :icon="Trash2"
            @click="remove(row.full)"
          />
        </template>
      </Table>
      <Empty v-else title="还没有客户端令牌" />
    </div>
  </Card>
</template>
