<script setup lang="ts">
import { Dialog } from 'fuxsto-design'

defineProps<{
  open: boolean
  title?: string
  description?: string
  size?: 'sm' | 'md' | 'lg' | 'xl'
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
}>()
</script>

<template>
  <Dialog
    :open="open"
    :title="title"
    :description="description"
    :size="size || 'md'"
    :show-confirm="false"
    :show-cancel="false"
    closable
    @update:open="emit('update:open', $event)"
  >
    <slot />
    <template #footer>
      <slot name="footer" />
    </template>
  </Dialog>
</template>
