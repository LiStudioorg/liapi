<script setup lang="ts">
import { Button, Input } from 'fuxsto-design'
import { KeyRound, Loader2, Eye, EyeOff, LogIn, TriangleAlert } from 'lucide-vue-next'
import { useApi } from '~/composables/useApi'

definePageMeta({ layout: 'auth' })

const { token, setToken, loadToken, request } = useApi()
const route = useRoute()

const value = ref('')
const show = ref(false)
const busy = ref(false)
const error = ref('')

onMounted(() => {
  loadToken()
  value.value = token.value
})

function targetPath(): string {
  const redirect = route.query.redirect
  const path = Array.isArray(redirect) ? redirect[0] : redirect
  if (typeof path === 'string' && path.startsWith('/') && path !== '/login') {
    return path
  }
  return '/'
}

async function submit() {
  error.value = ''
  const t = value.value.trim()
  if (!t) {
    error.value = '请输入 admin token'
    return
  }
  busy.value = true
  // Validate before storing so a wrong token never overwrites a good one.
  const prev = token.value
  setToken(t)
  try {
    await request('/overview')
    await navigateTo(targetPath(), { replace: true })
  } catch (e) {
    const err = e as Error & { unauthorized?: boolean }
    if (err.unauthorized) {
      setToken(prev)
      error.value = 'admin token 不正确或已失效'
    } else {
      error.value = err.message || '登录失败'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <form class="space-y-4" @submit.prevent="submit">
    <div class="space-y-1.5">
      <label class="text-xs font-medium text-muted-foreground">Admin Token</label>
      <div class="relative">
        <KeyRound
          :size="15"
          class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          v-model="value"
          :type="show ? 'text' : 'password'"
          placeholder="adm-..."
          autofocus
          autocomplete="current-password"
          class="pl-9 pr-9"
          @keydown.enter.prevent="submit"
        />
        <button
          type="button"
          class="absolute right-2 top-1/2 grid h-7 w-7 -translate-y-1/2 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          :title="show ? '隐藏' : '显示'"
          @click="show = !show"
        >
          <component :is="show ? EyeOff : Eye" :size="15" />
        </button>
      </div>
    </div>

    <Transition name="fade">
      <div
        v-if="error"
        class="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
      >
        <TriangleAlert :size="14" class="mt-0.5 shrink-0" />
        <span>{{ error }}</span>
      </div>
    </Transition>

    <Button type="submit" class="w-full" :disabled="busy">
      <component
        :is="busy ? Loader2 : LogIn"
        :size="16"
        :class="busy ? 'animate-spin' : ''"
      />
      {{ busy ? '验证中…' : '登录' }}
    </Button>
  </form>
</template>
