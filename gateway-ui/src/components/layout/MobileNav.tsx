import { useEffect, useState } from "react";
import { Menu, X, Bot } from "lucide-react";
import { cn } from "@/lib/utils";
import { NAV, type PageKey } from "@/components/layout/Sidebar";

/**
 * Mobile navigation drawer.
 *
 * The desktop sidebar is a fixed 240px column, which leaves almost nothing on
 * a phone. Below `lg` the same NAV items move into an off-canvas drawer so the
 * content column can use the full width and scroll normally.
 */
export function MobileNav({ page, onNavigate }: { page: PageKey; onNavigate: (p: PageKey) => void }) {
  const [open, setOpen] = useState(false);
  const title = NAV.find((n) => n.key === page)?.label ?? "";

  // Close on navigation and lock background scroll only while the drawer is
  // open — never on the page itself.
  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [open]);

  return (
    <>
      <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b bg-background/85 px-3 backdrop-blur lg:hidden">
        <button
          onClick={() => setOpen(true)}
          aria-label="打开导航菜单"
          className="-ml-1 flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <Menu className="h-5 w-5" />
        </button>
        <span className="text-sm font-semibold">{title}</span>
      </header>

      {open && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div className="absolute inset-0 bg-zinc-950/50 backdrop-blur-[2px]" onClick={() => setOpen(false)} />
          <div className="absolute inset-y-0 left-0 flex w-64 max-w-[80vw] flex-col border-r bg-card shadow-xl">
            <div className="flex h-14 items-center gap-2.5 border-b px-4">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
                <Bot className="h-4 w-4" />
              </div>
              <div className="leading-tight">
                <div className="text-sm font-semibold">liapi</div>
              </div>
              <button
                onClick={() => setOpen(false)}
                aria-label="关闭导航菜单"
                className="ml-auto flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-accent"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
              {NAV.map((item) => {
                const Icon = item.icon;
                const active = page === item.key;
                return (
                  <button
                    key={item.key}
                    onClick={() => {
                      onNavigate(item.key);
                      setOpen(false);
                    }}
                    className={cn(
                      "flex w-full items-center gap-2.5 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors",
                      active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                    )}
                  >
                    <Icon className="h-4 w-4" />
                    {item.label}
                  </button>
                );
              })}
            </nav>
            <div className="border-t p-4">
              <div className="flex items-center gap-2 rounded-lg bg-secondary px-3 py-2.5">
                <span className="h-2 w-2 rounded-full bg-emerald-500" />
                <span className="text-xs font-medium">Gateway 在线</span>
                <span className="ml-auto font-mono text-[10px] text-muted-foreground">v1.0</span>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
