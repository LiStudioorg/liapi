<script setup lang="ts">
defineProps<{ label: string; value: string | number }>()
</script>

<template>
  <div class="rounded-xl border border-border bg-muted/40 px-3 py-2.5">
    <div class="text-[11px] text-muted-foreground">{{ label }}</div>
    <div class="mt-0.5 truncate text-lg font-semibold">{{ value }}</div>
  </div>
</template>
