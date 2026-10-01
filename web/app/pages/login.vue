<script setup lang="ts">
import { Button, Input } from 'fuxsto-design'
import { User, Loader2, Eye, EyeOff, LogIn, TriangleAlert, KeyRound } from 'lucide-vue-next'
import { useApi } from '~/composables/useApi'

definePageMeta({ layout: 'auth' })

const { login } = useApi()
const route = useRoute()

const username = ref('')
const password = ref('')
const show = ref(false)
const busy = ref(false)
const error = ref('')

onMounted(() => {
  // Prefill the username only; never store/echo the password.
  username.value = localStorage.getItem('liapi_admin_username') || ''
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
  const u = username.value.trim()
  if (!u || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  busy.value = true
  try {
    await login(u, password.value)
    localStorage.setItem('liapi_admin_username', u)
    await navigateTo(targetPath(), { replace: true })
  } catch (e) {
    error.value = (e as Error).message || '登录失败'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <form class="space-y-4" @submit.prevent="submit">
    <div class="space-y-1.5">
      <label class="text-xs font-medium text-muted-foreground">用户名</label>
      <div class="relative">
        <User
          :size="15"
          class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          v-model="username"
          type="text"
          placeholder="admin"
          autofocus
          autocomplete="username"
          class="pl-9"
          @keydown.enter.prevent="submit"
        />
      </div>
    </div>

    <div class="space-y-1.5">
      <label class="text-xs font-medium text-muted-foreground">密码</label>
      <div class="relative">
        <KeyRound
          :size="15"
          class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          v-model="password"
          :type="show ? 'text' : 'password'"
          placeholder="••••••••"
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
      {{ busy ? '登录中…' : '登录' }}
    </Button>
  </form>
</template>
