<script setup lang="ts">
import { Gauge } from 'lucide-vue-next'

const dark = ref(true)

onMounted(() => {
  const saved = localStorage.getItem('liapi_theme')
  dark.value = saved ? saved === 'dark' : true
  document.documentElement.classList.toggle('dark', dark.value)
})
</script>

<template>
  <div
    class="relative flex min-h-screen items-center justify-center overflow-hidden bg-background px-4 text-foreground"
  >
    <!-- Ambient brand glow; purely decorative. -->
    <div
      class="pointer-events-none absolute -top-40 left-1/2 h-96 w-96 -translate-x-1/2 rounded-full bg-primary/20 blur-3xl"
    />
    <div
      class="pointer-events-none absolute -bottom-40 right-1/4 h-80 w-80 rounded-full bg-primary/10 blur-3xl"
    />

    <div class="relative w-full max-w-sm">
      <div class="mb-6 flex flex-col items-center gap-3 text-center">
        <div
          class="grid h-12 w-12 place-items-center rounded-2xl bg-primary text-primary-foreground shadow-lg shadow-primary/20"
        >
          <Gauge :size="24" />
        </div>
        <div>
          <div class="text-lg font-semibold tracking-tight">Liapi</div>
          <div class="text-xs text-muted-foreground">API Gateway 管理台</div>
        </div>
      </div>

      <div
        class="rounded-2xl border border-border bg-card/60 p-6 shadow-xl backdrop-blur-sm"
      >
        <slot />
      </div>

      <p class="mt-6 text-center text-[11px] text-muted-foreground">
        admin token 仅保存在本机浏览器，不会上传
      </p>
    </div>
  </div>
</template>
