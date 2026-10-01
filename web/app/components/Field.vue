<script setup lang="ts">
defineProps<{ label: string }>()
</script>

<template>
  <div class="mb-3">
    <label class="mb-1.5 block text-xs font-medium text-muted-foreground">
      {{ label }}
    </label>
    <slot />
  </div>
</template>
