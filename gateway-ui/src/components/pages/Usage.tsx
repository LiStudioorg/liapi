import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from "recharts";
import { AlertTriangle, ChevronDown, Loader2, RefreshCw } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useToast } from "@/context/theme";
import { api, type DailyPoint, type StatsSummary, type UsageBucket } from "@/lib/api";

const RANGES = ["今天", "1天", "7天", "14天", "30天"] as const;
type Range = (typeof RANGES)[number];
const RANGE_HOURS: Record<Range, number> = { 今天: 24, "1天": 24, "7天": 24 * 7, "14天": 24 * 14, "30天": 24 * 30 };

/** 中文数量级 */
function zh(n: number, digits = 2): string {
  if (n >= 1e8) return (n / 1e8).toFixed(digits).replace(/\.?0+$/, "") + " 亿";
  if (n >= 1e4) return (n / 1e4).toFixed(digits).replace(/\.?0+$/, "") + " 万";
  return n.toLocaleString("en-US");
}
function label(n: number): string {
  if (n >= 1e8) return (n / 1e8).toFixed(2) + " 亿";
  return (n / 1e4).toFixed(2) + " 万";
}

function PointLabel(props: any) {
  const { x, y, value, index } = props;
  if (x == null || y == null || !value) return null;
  const dy = index % 2 === 1 ? -18 : -6;
  return (
    <text x={x} y={y + dy} textAnchor="middle" className="fill-muted-foreground" fontSize={11}>
      {label(value)}
    </text>
  );
}

export default function Usage() {
  const toast = useToast();
  const [range, setRange] = useState<Range>("7天");
  const [model, setModel] = useState("all");
  const [source, setSource] = useState("all");
  const [countdown, setCountdown] = useState(60);

  const [summary, setSummary] = useState<StatsSummary | null>(null);
  const [usage, setUsage] = useState<UsageBucket[]>([]);
  const [daily, setDaily] = useState<DailyPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const hours = RANGE_HOURS[range];
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [s, u, d] = await Promise.all([
        api.statsSummary(hours),
        api.statsUsage(hours, model, source),
        api.statsDaily(Math.max(1, Math.round(hours / 24))),
      ]);
      setSummary(s);
      setUsage(u);
      setDaily(d);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
      setCountdown(60);
    }
  }, [hours, model, source]);

  useEffect(() => {
    void load();
  }, [load]);

  // 60s 自动刷新倒计时：归零即重新拉取
  useEffect(() => {
    timer.current = setInterval(() => {
      setCountdown((c) => {
        if (c <= 1) {
          void load();
          return 60;
        }
        return c - 1;
      });
    }, 1000);
    return () => {
      if (timer.current) clearInterval(timer.current);
    };
  }, [load]);

  const chart: DailyPoint[] = useMemo(() => daily, [daily]);
  const totals = summary ?? { calls: 0, tokens: 0, in_tokens: 0, out_tokens: 0, cost: 0, avg_latency: 0, errors: 0 };
  const grand = Math.max(totals.tokens, 1);

  const modelOptions = useMemo(() => ["all", ...new Set(usage.map((u) => u.model).filter(Boolean))], [usage]);
  const sourceOptions = useMemo(() => ["all", ...new Set(usage.map((u) => u.upstream).filter(Boolean))], [usage]);

  const exportCsv = () => {
    const rows = [
      ["时段", "来源", "模型", "调用", "输入Token", "输出Token", "费用"],
      ...usage.map((u) => [u.bucket, u.upstream, u.model, String(u.calls), String(u.in_tokens), String(u.out_tokens), String(u.cost)]),
    ];
    const csv = "\ufeff" + rows.map((r) => r.join(",")).join("\n");
    const url = URL.createObjectURL(new Blob([csv], { type: "text/csv;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `usage-${range}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    toast("CSV 已导出");
  };

  return (
    <div className="flex w-full flex-col gap-4">
      {/* ── 工具栏 ── */}
      <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex flex-wrap items-center gap-1 rounded-lg bg-muted p-1">
            {RANGES.map((r) => (
              <button
                key={r}
                onClick={() => setRange(r)}
                className={cn(
                  "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                  range === r ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
                )}
              >
                {r}
              </button>
            ))}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Select value={model} onValueChange={setModel}>
            <SelectTrigger className="w-full sm:w-[150px]"><SelectValue /></SelectTrigger>
            <SelectContent>
              {modelOptions.map((m) => (
                <SelectItem key={m} value={m}>{m === "all" ? "全部模型" : m}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={source} onValueChange={setSource}>
            <SelectTrigger className="w-full sm:w-[150px]"><SelectValue /></SelectTrigger>
            <SelectContent>
              {sourceOptions.map((s) => (
                <SelectItem key={s} value={s}>{s === "all" ? "全部来源" : s}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <button
            onClick={() => void load()}
            className="inline-flex items-center gap-1.5 rounded-md border bg-card px-3 py-2 text-sm font-medium shadow-sm transition-colors hover:bg-accent"
          >
            {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            {countdown}s
          </button>
        </div>
      </div>

      {error && (
        <Card className="border-red-500/40">
          <CardContent className="flex items-center gap-3 p-4">
            <AlertTriangle className="h-4 w-4 shrink-0 text-red-500" />
            <div className="text-sm text-muted-foreground">{error}</div>
          </CardContent>
        </Card>
      )}

      {/* ── 概览 ── */}
      <Card>
        <CardContent className="flex flex-col gap-5 p-5 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="text-sm text-muted-foreground">总 Token 消耗</div>
            <div className="mt-1 flex flex-wrap items-baseline gap-2">
              <span className="text-3xl font-semibold tabular-nums tracking-tight sm:text-4xl">
                {totals.tokens.toLocaleString("en-US")}
              </span>
              <span className="text-sm text-muted-foreground">≈ {zh(totals.tokens)}</span>
            </div>
          </div>
          <div className="flex shrink-0 gap-8 sm:gap-12">
            <div>
              <div className="text-sm text-muted-foreground">API 调用次数</div>
              <div className="mt-1 text-2xl font-semibold tabular-nums">{totals.calls.toLocaleString("en-US")}</div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">预估总花费</div>
              <div className="mt-1 text-2xl font-semibold tabular-nums">
                ${totals.cost.toFixed(4)}
                <span className="ml-1 text-sm font-normal text-muted-foreground">
                  ≈ ¥{(totals.cost * 7.2).toFixed(2)}
                </span>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* ── 细分 ── */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <BreakdownCard
          title="输入" total={totals.in_tokens} pct={((totals.in_tokens / grand) * 100).toFixed(1) + "%"}
          parts={[{ label: "普通输入", value: totals.in_tokens }]}
        />
        <BreakdownCard
          title="输出" total={totals.out_tokens} pct={((totals.out_tokens / grand) * 100).toFixed(1) + "%"}
          parts={[{ label: "内容生成", value: totals.out_tokens }]}
        />
        <BreakdownCard
          title="调用" total={totals.calls} pct={`均 ${totals.avg_latency} ms`} unit="次"
          parts={[{ label: "错误响应", value: totals.errors }]}
        />
      </div>

      {/* ── 走势 ── */}
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <CardTitle>每日消耗走势</CardTitle>
          <span className="text-sm text-muted-foreground">{zh(totals.tokens)} Tokens</span>
        </CardHeader>
        <CardContent className="p-2 pb-4 sm:p-4">
          <div className="h-[400px] w-full">
            {chart.length === 0 ? (
              <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
                <span>暂无调用记录</span>
                <span className="text-xs">用设备 Token 调一次 /v1/chat/completions 后这里会出现数据</span>
              </div>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chart} margin={{ top: 28, right: 12, left: 4, bottom: 4 }}>
                  <defs>
                    <linearGradient id="usageFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#a855f7" stopOpacity={0.5} />
                      <stop offset="95%" stopColor="#a855f7" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
                  <XAxis dataKey="day" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} minTickGap={8} />
                  <YAxis
                    tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={52}
                    tickFormatter={(v: number) => (v >= 1e8 ? (v / 1e8).toFixed(1) + "亿" : (v / 1e4).toFixed(0) + "万")}
                  />
                  <Tooltip
                    formatter={(v: number) => [v.toLocaleString("en-US") + " tokens", "消耗"]}
                    contentStyle={{ borderRadius: 10, border: "1px solid hsl(var(--border))", fontSize: 12 }}
                  />
                  <Area
                    type="monotone" dataKey="tokens" stroke="#a855f7" strokeWidth={2}
                    fill="url(#usageFill)" dot={{ r: 3, fill: "#a855f7", strokeWidth: 0 }}
                    activeDot={{ r: 5 }} label={<PointLabel />}
                  />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </div>

          {/* 明细表 */}
          <div className="mt-3 overflow-x-auto border-t pt-3">
            {usage.length > 0 && (
              <table className="w-full min-w-[640px] text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground">
                    <th className="py-2 font-medium">时段</th>
                    <th className="py-2 font-medium">来源</th>
                    <th className="py-2 font-medium">模型</th>
                    <th className="py-2 text-right font-medium">调用</th>
                    <th className="py-2 text-right font-medium">输入</th>
                    <th className="py-2 text-right font-medium">输出</th>
                  </tr>
                </thead>
                <tbody>
                  {usage.slice(0, 20).map((u, i) => (
                    <tr key={i} className="border-t">
                      <td className="py-2 font-mono text-xs text-muted-foreground">{u.bucket}</td>
                      <td className="max-w-[180px] truncate py-2 font-mono text-xs">{u.upstream}</td>
                      <td className="py-2 font-mono text-xs">{u.model}</td>
                      <td className="py-2 text-right tabular-nums">{u.calls}</td>
                      <td className="py-2 text-right tabular-nums">{u.in_tokens.toLocaleString("en-US")}</td>
                      <td className="py-2 text-right tabular-nums">{u.out_tokens.toLocaleString("en-US")}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            <span className="text-xs text-muted-foreground">
              {model === "all" ? "全部模型" : model} · {source === "all" ? "全部来源" : source} · {usage.length} 条明细
            </span>
            <div className="flex items-center gap-2">
              <button onClick={exportCsv} className="inline-flex items-center gap-1.5 rounded-md border bg-card px-3 py-1.5 text-xs font-medium shadow-sm hover:bg-accent">
                导出 CSV
              </button>
              <button className="inline-flex items-center gap-1.5 rounded-md border bg-card px-3 py-1.5 text-xs font-medium shadow-sm hover:bg-accent">
                按小时 <ChevronDown className="h-3 w-3" />
              </button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function BreakdownCard({
  title, total, pct, parts, unit = "",
}: {
  title: string;
  total: number;
  pct: string;
  parts: { label: string; value: number }[];
  unit?: string;
}) {
  return (
    <Card className="min-w-0">
      <CardContent className="p-5">
        <div className="flex items-start justify-between">
          <span className="text-sm font-medium text-muted-foreground">{title}</span>
          <span className="text-sm font-semibold text-violet-500">{pct}</span>
        </div>
        <div className="mt-2 text-2xl font-semibold tabular-nums sm:text-3xl">
          {unit === "次" ? total.toLocaleString("en-US") + " 次" : zh(total)}
        </div>
        <div className="mt-3 space-y-1">
          {parts.map((p) => (
            <div key={p.label} className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-violet-500" />
              {p.label}: {unit === "次" ? p.value.toLocaleString("en-US") + " 次" : zh(p.value)}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
