import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { CheckCircle2, Info, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";

type Theme = "light" | "dark";
const ThemeCtx = createContext<{ theme: Theme; toggle: () => void }>({ theme: "light", toggle: () => {} });

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => (localStorage.getItem("gw-theme") as Theme) || "light");
  useEffect(() => {
    document.documentElement.classList.toggle("dark", theme === "dark");
    localStorage.setItem("gw-theme", theme);
  }, [theme]);
  const toggle = useCallback(() => setTheme((t) => (t === "light" ? "dark" : "light")), []);
  return <ThemeCtx.Provider value={{ theme, toggle }}>{children}</ThemeCtx.Provider>;
}

export const useTheme = () => useContext(ThemeCtx);

/* ------------------------------ 轻量 Toast ------------------------------ */

type ToastItem = { id: number; msg: string; kind: "ok" | "warn" | "err" };
const ToastCtx = createContext<(msg: string, kind?: ToastItem["kind"]) => void>(() => {});
export const useToast = () => useContext(ToastCtx);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const push = useCallback((msg: string, kind: ToastItem["kind"] = "ok") => {
    const id = Date.now() + Math.random();
    setItems((l) => [...l, { id, msg, kind }]);
    setTimeout(() => setItems((l) => l.filter((t) => t.id !== id)), 2600);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed bottom-6 right-6 z-[100] flex flex-col gap-2">
        {items.map((t) => (
          <div
            key={t.id}
            className={cn(
              "animate-fade-up flex items-center gap-2 rounded-lg border bg-card px-3.5 py-2.5 text-sm shadow-lg",
              t.kind === "ok" && "border-emerald-500/30",
              t.kind === "warn" && "border-amber-500/30",
              t.kind === "err" && "border-red-500/30"
            )}
          >
            {t.kind === "ok" && <CheckCircle2 className="h-4 w-4 text-emerald-500" />}
            {t.kind === "warn" && <Info className="h-4 w-4 text-amber-500" />}
            {t.kind === "err" && <XCircle className="h-4 w-4 text-red-500" />}
            {t.msg}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}
