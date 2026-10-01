import { useEffect, useState } from "react";
import {
  Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from "recharts";
import {
  Activity, AlertTriangle, Banknote, Coins, Copy, Cpu, Gauge, Loader2, Radio, Zap,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useToast } from "@/context/theme";
import { api, type DailyPoint, type LogRow, type ModelStat, type StatsSummary } from "@/lib/api";

/** 主题色板（按索引循环） */
const COLORS = ["#3b82f6", "#8b5cf6", "#10b981", "#f59e0b", "#ef4444", "#06b6d4"];

export default function Dashboard() {
  const toast = useToast();
  const [summary, setSummary] = useState<StatsSummary | null>(null);
  const [logs, setLogs] = useState<LogRow[]>([]);
  const [daily, setDaily] = useState<DailyPoint[]>([]);
  const [models, setModels] = useState<ModelStat[]>([]);
  const [loading, setLoading] = useState(true);
  const [stream, setStream] = useState(false);

  const load = async () => {
    try {
      const [s, l, d, m] = await Promise.all([
        api.statsSummary(24),
        api.listLogs(12),
        api.statsDaily(7),
        api.statsModels(),
      ]);
      setSummary(s);
      setLogs(l);
      setDaily(d);
      setModels(m);
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 实时日志流：真实轮询
  useEffect(() => {
    if (!stream) return;
    const t = setInterval(() => {
      api.listLogs(12).then(setLogs).catch(() => {});
      api.statsSummary(24).then(setSummary).catch(() => {});
    }, 3000);
    return () => clearInterval(t);
  }, [stream]);

  const s = summary ?? { calls: 0, in_tokens: 0, out_tokens: 0, tokens: 0, cost: 0, avg_latency: 0, errors: 0 };
  const base = import.meta.env.VITE_API_BASE || window.location.origin;

  const STATS = [
    { label: "今日总请求", value: s.calls.toLocaleString("en-US"), icon: Activity, tint: "text-blue-500" },
    { label: "平均延迟", value: `${s.avg_latency} ms`, icon: Zap, tint: "text-violet-500" },
    { label: "今日输入 Token", value: s.in_tokens.toLocaleString("en-US"), icon: Cpu, tint: "text-emerald-500" },
    { label: "今日输出 Token", value: s.out_tokens.toLocaleString("en-US"), icon: Cpu, tint: "text-amber-500" },
    { label: "今日总 Token", value: s.tokens.toLocaleString("en-US"), icon: Coins, tint: "text-cyan-500" },
    { label: "今日预估费用", value: `$${s.cost.toFixed(4)}`, icon: Banknote, tint: "text-rose-500" },
  ];

  const statusBadge = (code: number | null) => {
    if (code === 200) return <Badge variant="success">200</Badge>;
    if (code === 429) return <Badge variant="warning">429</Badge>;
    return <Badge variant="destructive">{code ?? "ERR"}</Badge>;
  };

  return (
    <div className="space-y-6">
      {/* 接入地址 */}
      <Card className="overflow-hidden border-violet-500/20">
        <div className="bg-gradient-to-r from-violet-600 via-purple-600 to-fuchsia-600 p-5 text-white">
          <div className="flex flex-wrap items-center gap-3">
            <div className="min-w-0">
              <div className="text-xs font-medium uppercase tracking-wider text-white/70">网关接入地址</div>
              <div className="mt-1 truncate font-mono text-lg font-semibold">{base}/v1</div>
            </div>
            <Badge className="border-white/30 bg-white/15 text-white">OpenAI 兼容</Badge>
            <Button
              size="sm" variant="secondary"
              className="ml-auto bg-white/15 text-white backdrop-blur hover:bg-white/25"
              onClick={async () => { await navigator.clipboard.writeText(`${base}/v1`); toast("Base URL 已复制"); }}
            >
              <Copy className="h-3.5 w-3.5" /> 复制 Base URL
            </Button>
          </div>
        </div>
      </Card>

      {/* 6 项统计 */}
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-6">
        {STATS.map((it) => {
          const Icon = it.icon;
          return (
            <Card key={it.label} className="p-4">
              <div className="flex items-center justify-between">
                <span className="truncate text-xs text-muted-foreground">{it.label}</span>
                <Icon className={`h-4 w-4 shrink-0 ${it.tint}`} />
              </div>
              <div className="mt-2 text-xl font-semibold tabular-nums">
                {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : it.value}
              </div>
            </Card>
          );
        })}
      </div>

      {/* 图表 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
<Card className="min-w-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><Activity className="h-3.5 w-3.5" /> 近 7 日调用趋势</CardTitle>
            <CardDescription>数据来自 logs 表实时聚合</CardDescription>
          </CardHeader>
          <CardContent className="h-56">
            {daily.length === 0 ? (
              <Empty />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={daily} margin={{ left: -18, right: 6 }}>
                  <defs>
                    <linearGradient id="g1" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#3b82f6" stopOpacity={0.35} />
                      <stop offset="100%" stopColor="#3b82f6" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
                  <XAxis dataKey="day" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                  <Tooltip contentStyle={{ borderRadius: 10, fontSize: 12 }} />
                  <Area type="monotone" dataKey="calls" stroke="#3b82f6" strokeWidth={2} fill="url(#g1)" name="请求数" />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

<Card className="min-w-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><Gauge className="h-3.5 w-3.5" /> 模型延迟评测</CardTitle>
            <CardDescription>按日志平均耗时</CardDescription>
          </CardHeader>
          <CardContent className="h-56">
            {models.length === 0 ? (
              <Empty />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={models.slice(0, 5)} layout="vertical" margin={{ left: 40, right: 10 }}>
                  <XAxis type="number" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} unit="ms" />
                  <YAxis type="category" dataKey="model" width={110} tick={{ fontSize: 10 }} axisLine={false} tickLine={false} />
                  <Tooltip contentStyle={{ borderRadius: 10, fontSize: 12 }} />
                  <Bar dataKey="avg_latency" name="平均延迟" radius={[0, 6, 6, 0]} barSize={12}>
                    {models.slice(0, 5).map((_, i) => (
                      <Cell key={i} fill={COLORS[i % COLORS.length]} />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>
      </div>

      {/* 请求日志 */}
      <Card>
        <CardHeader className="flex-row items-center gap-3 space-y-0">
          <div>
            <CardTitle>网关请求日志</CardTitle>
            <CardDescription>最新 {logs.length} 条{stream ? "（每 3 秒刷新）" : ""}</CardDescription>
          </div>
          <Button
            size="sm"
            variant={stream ? "destructive" : "default"}
            className="ml-auto"
            onClick={() => { setStream((v) => !v); toast(stream ? "已停止实时日志流" : "实时日志流已开启", stream ? "warn" : "ok"); }}
          >
            <Radio className="h-3.5 w-3.5" />
            {stream ? "停止实时日志流" : "开启实时日志流"}
          </Button>
        </CardHeader>
        <CardContent className="overflow-x-auto">
          <Table className="min-w-[720px]">
            <TableHeader>
              <TableRow>
                <TableHead>时间</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>模型</TableHead>
                <TableHead>上游地址</TableHead>
                <TableHead className="text-right">Token 数</TableHead>
                <TableHead className="text-right">耗时</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="whitespace-nowrap font-mono text-xs text-muted-foreground">{l.created_at}</TableCell>
                  <TableCell>{statusBadge(l.status)}</TableCell>
                  <TableCell className="font-mono text-xs">{l.model ?? "—"}</TableCell>
                  <TableCell className="max-w-[240px] truncate font-mono text-xs text-muted-foreground">{l.upstream ?? "—"}</TableCell>
                  <TableCell className="text-right tabular-nums">{(l.in_tokens + l.out_tokens).toLocaleString("en-US")}</TableCell>
                  <TableCell className="text-right tabular-nums text-muted-foreground">{l.latency_ms} ms</TableCell>
                </TableRow>
              ))}
              {!loading && logs.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                    还没有调用记录 —— 在 Playground 发一条消息试试
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}

function Empty() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      <AlertTriangle className="mr-2 h-4 w-4" /> 暂无数据
    </div>
  );
}
