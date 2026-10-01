import { useState } from "react";
import { Sidebar, type PageKey, NAV } from "@/components/layout/Sidebar";
import { MobileNav } from "@/components/layout/MobileNav";
import { Topbar } from "@/components/layout/Topbar";
import { ThemeProvider, ToastProvider } from "@/context/theme";
import Dashboard from "@/components/pages/Dashboard";
import Providers from "@/components/pages/Providers";
import Models from "@/components/pages/Models";
import Playground from "@/components/pages/Playground";
import Devices from "@/components/pages/Devices";
import Usage from "@/components/pages/Usage";
import Settings from "@/components/pages/Settings";

const PAGES: Record<PageKey, React.ComponentType> = {
  dashboard: Dashboard,
  providers: Providers,
  models: Models,
  playground: Playground,
  devices: Devices,
  usage: Usage,
  settings: Settings,
};

export default function App() {
  const [page, setPage] = useState<PageKey>("dashboard");
  const title = NAV.find((n) => n.key === page)?.label ?? "";
  const Page = PAGES[page];
  return (
    <ThemeProvider>
      <ToastProvider>
        {/* The page itself scrolls (body-level scroll). Nothing here sets
            overflow:hidden, so mobile browsers can pan the whole page. */}
        <div className="min-h-dvh bg-background">
          <Sidebar page={page} onNavigate={setPage} />
          <div className="lg:pl-60">
            <MobileNav page={page} onNavigate={setPage} />
            <div className="hidden lg:block">
              <Topbar title={title} />
            </div>
            {/* min-w-0 是防挤压的关键：避免固定侧边栏 + grid/flex 子项
                互相撑宽导致内容"被挤出来"。max-w-7xl+mx-auto 在窄屏会收窄。 */}
            <main className="mx-auto min-w-0 max-w-7xl p-4 sm:p-6" key={page}>
              <div className="min-w-0">
                <Page />
              </div>
            </main>
          </div>
        </div>
      </ToastProvider>
    </ThemeProvider>
  );
}
