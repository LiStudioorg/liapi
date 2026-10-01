<script setup lang="ts">
import { Button, Input } from 'fuxsto-design'
import { User, Loader2, Eye, EyeOff, LogIn, TriangleAlert, KeyRound, Info } from 'lucide-vue-next'
import { useApi } from '~/composables/useApi'

definePageMeta({ layout: 'auth' })

interface LoginInfo {
  login_enabled: boolean
  password_set: boolean
}

const { login, setToken, clearToken, request } = useApi()
const route = useRoute()

const info = ref<LoginInfo | null>(null)
const username = ref('')
const password = ref('')
const show = ref(false)
const busy = ref(false)
const error = ref('')

// Token-direct mode: used when username/password login is disabled (fresh
// installs ship without credentials) or when the user prefers the token.
const tokenMode = ref(false)
const adminToken = ref('')

onMounted(async () => {
  try {
    info.value = await request<LoginInfo>('/login-info')
  } catch {
    // Endpoint unreachable (old server?) — assume classic password form.
    info.value = { login_enabled: true, password_set: true, admin_username: 'admin' }
  }
  if (info.value && !info.value.login_enabled) tokenMode.value = true
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
  if (tokenMode.value) {
    const t = adminToken.value.trim()
    if (!t) {
      error.value = '请输入 Admin Token'
      return
    }
    busy.value = true
    try {
      // Store, then verify against /me; a wrong token bounces back here.
      setToken(t)
      await request('/me')
      await navigateTo(targetPath(), { replace: true })
    } catch (e) {
      error.value = (e as Error).message === 'unauthorized' ? 'Token 无效' : (e as Error).message || '验证失败'
      clearToken()
    } finally {
      busy.value = false
    }
    return
  }
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
    <!-- Login disabled: only the static admin token can unlock the console. -->
    <Transition name="fade">
      <div
        v-if="tokenMode && info && !info.password_set"
        class="flex items-start gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground"
      >
        <Info :size="14" class="mt-0.5 shrink-0" />
        <span>
          尚未配置用户名/密码登录。可先用启动日志里的 Admin Token 登录；之后在
          「配置 → 管理台与鉴权」中开启登录并设置自己的账号密码。也可编辑配置文件：
          <code class="rounded bg-muted px-1">login_enabled: true</code> +
          <code class="rounded bg-muted px-1">admin_password: "你的密码"</code>
        </span>
      </div>
    </Transition>

    <template v-if="tokenMode">
      <div class="space-y-1.5">
        <label class="text-xs font-medium text-muted-foreground">Admin Token</label>
        <div class="relative">
          <KeyRound
            :size="15"
            class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            v-model="adminToken"
            :type="show ? 'text' : 'password'"
            placeholder="粘贴 admin_token（启动日志或配置文件）"
            autofocus
            autocomplete="off"
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
      <button
        v-if="info?.password_set"
        type="button"
        class="w-full text-center text-xs text-primary hover:underline"
        @click="tokenMode = false"
      >
        改用用户名密码登录
      </button>
    </template>

    <template v-else>
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

      <button
        type="button"
        class="w-full text-center text-xs text-muted-foreground hover:text-foreground"
        @click="tokenMode = true"
      >
        使用 Admin Token 登录
      </button>
    </template>

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
      {{ busy ? '登录中…' : tokenMode ? '进入管理台' : '登录' }}
    </Button>
  </form>
</template>
