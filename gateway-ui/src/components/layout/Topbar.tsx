import { LogOut, MonitorSmartphone } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTheme, useToast } from "@/context/theme";

export function Topbar({ title }: { title: string }) {
  const { theme, toggle } = useTheme();
  const toast = useToast();
  return (
    <header className="sticky top-0 z-20 flex h-14 items-center border-b bg-background/80 px-6 backdrop-blur">
      <h1 className="text-sm font-semibold">{title}</h1>
      <div className="ml-auto flex items-center gap-2">
        <Button
          variant="ghost"
          size="icon"
          title={theme === "light" ? "切换到暗色" : "切换到亮色"}
          onClick={toggle}
        >
          <MonitorSmartphone className="h-4 w-4" />
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => toast("已退出登录（Mock）", "warn")}
        >
          <LogOut className="h-3.5 w-3.5" />
          退出登录
        </Button>
      </div>
    </header>
  );
}
