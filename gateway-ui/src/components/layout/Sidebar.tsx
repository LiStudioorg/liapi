import {
  BarChart3, Blocks, Bot, KeyRound, LayoutDashboard, MessagesSquare, Route, Settings2,
} from "lucide-react";
import { cn } from "@/lib/utils";

export const NAV = [
  { key: "dashboard", label: "仪表盘", icon: LayoutDashboard },
  { key: "providers", label: "Providers", icon: Blocks },
  { key: "models", label: "Models", icon: Route },
  { key: "playground", label: "Playground", icon: MessagesSquare },
  { key: "devices", label: "设备 Token", icon: KeyRound },
  { key: "usage", label: "用量记录", icon: BarChart3 },
  { key: "settings", label: "系统设置", icon: Settings2 },
] as const;

export type PageKey = (typeof NAV)[number]["key"];

export function Sidebar({ page, onNavigate }: { page: PageKey; onNavigate: (p: PageKey) => void }) {
  return (
    <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col border-r bg-card lg:flex">
      <div className="flex h-14 items-center gap-2.5 border-b px-5">
        <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
          <Bot className="h-4 w-4" />
        </div>
        <div className="leading-tight">
          <div className="text-sm font-semibold">liapi</div>
        </div>
      </div>

      <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
        {NAV.map((item) => {
          const Icon = item.icon;
          const active = page === item.key;
          return (
            <button
              key={item.key}
              onClick={() => onNavigate(item.key)}
              className={cn(
                "flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                active
                  ? "bg-primary/10 text-primary"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              )}
            >
              <Icon className="h-4 w-4" strokeWidth={2} />
              {item.label}
              {active && <span className="ml-auto h-1.5 w-1.5 rounded-full bg-primary" />}
            </button>
          );
        })}
      </nav>

      <div className="border-t p-4">
        <div className="flex items-center gap-2 rounded-lg bg-secondary px-3 py-2.5">
          <span className="relative flex h-2 w-2">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
          </span>
          <span className="text-xs font-medium">Gateway 在线</span>
          <span className="ml-auto font-mono text-[10px] text-muted-foreground">v1.0</span>
        </div>
      </div>
    </aside>
  );
}
