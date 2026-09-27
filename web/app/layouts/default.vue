<script setup lang="ts">
import {
  LayoutDashboard,
  Server,
  KeyRound,
  Smartphone,
  BarChart3,
  ScrollText,
  Activity,
  Settings2,
  FlaskConical,
  Menu as MenuIcon,
  X,
  Moon,
  Sun,
  LogOut,
  CircleUser,
  Gauge,
} from 'lucide-vue-next'

const { token, loadToken, clearToken } = useApi()
const route = useRoute()
const router = useRouter()

const NAV = [
  { to: '/', label: '概览', icon: LayoutDashboard },
  { to: '/upstreams', label: '上游', icon: Server },
  { to: '/tokens', label: '令牌', icon: KeyRound },
  { to: '/devices', label: '设备', icon: Smartphone },
  { to: '/stats', label: '统计', icon: BarChart3 },
  { to: '/logs', label: '日志', icon: ScrollText },
  { to: '/health', label: '健康', icon: Activity },
  { to: '/config', label: '配置', icon: Settings2 },
  { to: '/debug', label: '调试', icon: FlaskConical },
]

const sidebarOpen = ref(false)
const dark = ref(true)

onMounted(() => {
  loadToken()
  const saved = localStorage.getItem('liapi_theme')
  dark.value = saved ? saved === 'dark' : true
  applyTheme()
  if (!token.value) goLogin()
})

function applyTheme() {
  const el = document.documentElement
  el.classList.toggle('dark', dark.value)
  if (import.meta.client) {
    localStorage.setItem('liapi_theme', dark.value ? 'dark' : 'light')
  }
}

function toggleTheme() {
  dark.value = !dark.value
  applyTheme()
}

// Auth gate: without a token the console is unusable, so send the user to
// the dedicated login page (no browser prompt) and preserve the target.
function goLogin() {
  router.push({ path: '/login', query: { redirect: route.fullPath } })
}

watch(
  () => route.path,
  () => {
    sidebarOpen.value = false
  },
)

const activeLabel = computed(
  () => NAV.find((n) => n.to === route.path)?.label || '概览',
)
</script>

<template>
  <div class="flex min-h-screen bg-background text-foreground">
    <!-- Sidebar -->
    <aside
      class="fixed inset-y-0 left-0 z-40 flex w-60 shrink-0 flex-col border-r border-border bg-background transition-transform duration-300 lg:static lg:translate-x-0"
      :class="sidebarOpen ? 'translate-x-0' : '-translate-x-full'"
    >
      <div class="flex h-16 items-center gap-2.5 border-b border-border px-5">
        <div
          class="grid h-8 w-8 place-items-center rounded-lg bg-primary text-primary-foreground"
        >
          <Gauge :size="18" />
        </div>
        <div class="leading-tight">
          <div class="text-sm font-semibold tracking-tight">Liapi</div>
          <div class="text-[11px] text-muted-foreground">API Gateway</div>
        </div>
      </div>

      <nav class="flex-1 space-y-1 overflow-y-auto p-3">
        <NuxtLink
          v-for="item in NAV"
          :key="item.to"
          :to="item.to"
          class="group flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-muted-foreground transition-all duration-200 hover:bg-muted hover:text-foreground"
          active-class="!bg-muted !text-foreground"
          :exact-active-class="item.to === '/' ? '!bg-muted !text-foreground' : ''"
        >
          <component
            :is="item.icon"
            :size="17"
            class="transition-transform duration-200 group-hover:scale-110"
          />
          <span>{{ item.label }}</span>
        </NuxtLink>
      </nav>

      <div class="border-t border-border p-3">
        <button
          class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          @click="clearToken(); goLogin()"
        >
          <LogOut :size="16" />
          <span>退出登录</span>
        </button>
      </div>
    </aside>

    <!-- Backdrop -->
    <Transition name="fade">
      <div
        v-if="sidebarOpen"
        class="fixed inset-0 z-30 bg-black/40 lg:hidden"
        @click="sidebarOpen = false"
      />
    </Transition>

    <!-- Main -->
    <div class="flex min-w-0 flex-1 flex-col">
      <header
        class="sticky top-0 z-20 flex h-16 items-center gap-3 border-b border-border bg-background/80 px-4 backdrop-blur-md sm:px-6"
      >
        <button
          class="grid h-9 w-9 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground lg:hidden"
          @click="sidebarOpen = !sidebarOpen"
        >
          <component :is="sidebarOpen ? X : MenuIcon" :size="19" />
        </button>

        <h1 class="text-base font-semibold tracking-tight">
          {{ activeLabel }}
        </h1>

        <div class="ml-auto flex items-center gap-2">
          <button
            class="grid h-9 w-9 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            :title="dark ? '切换到亮色' : '切换到暗色'"
            @click="toggleTheme"
          >
            <component :is="dark ? Sun : Moon" :size="18" />
          </button>
          <button
            class="flex items-center gap-2 rounded-lg border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            :title="token ? '切换 / 更新 token' : '登录'"
            @click="goLogin"
          >
            <CircleUser :size="15" />
            <span class="hidden sm:inline">
              {{ token ? '已认证' : '未认证' }}
            </span>
          </button>
        </div>
      </header>

      <main class="flex-1 p-4 sm:p-6">
        <slot />
      </main>
    </div>
  </div>
</template>
