import { useEffect, useState } from "react";
import {
  AlertTriangle, Camera, Copy, Database, ExternalLink, FileJson, Globe, History,
  Loader2, Plus, RotateCcw, Save, Send, ShieldCheck, Trash2, Wallet, Webhook, X,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/context/theme";
import { api, type AuditRow, type Settings as SettingsShape, type Upstream } from "@/lib/api";

export default function Settings() {
  const toast = useToast();
  const [cfg, setCfg] = useState<SettingsShape | null>(null);
  const [audit, setAudit] = useState<AuditRow[]>([]);
  const [upstreams, setUpstreams] = useState<Upstream[]>([]);
  const [saving, setSaving] = useState(false);
  const [ipDraft, setIpDraft] = useState("");
  const [hook, setHook] = useState("");
  const [base, setBase] = useState("");

  const load = async () => {
    try {
      const [c, a, u] = await Promise.all([api.getSettings(), api.listAudit(20), api.listUpstreams()]);
      setCfg(c);
      setAudit(a);
      setUpstreams(u);
      setHook(c.webhook_url ?? "");
    } catch (e: any) {
      toast(e.message, "err");
    }
  };

  useEffect(() => {
    setBase(import.meta.env.VITE_API_BASE || window.location.origin);
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /** 局部更新并立即落库 */
  const patch = async (kv: Record<string, any>) => {
    setCfg((c) => ({ ...(c ?? {}), ...kv }));
    setSaving(true);
    try {
      const next = await api.saveSettings(kv);
      setCfg(next);
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setSaving(false);
    }
  };

  const ips: string[] = Array.isArray(cfg?.ip_whitelist) ? cfg!.ip_whitelist : [];

  const CODE = {
    python: `from openai import OpenAI

client = OpenAI(
    base_url="${base}/v1",
    api_key="sk-cp-你的设备Token",
)

resp = client.chat.completions.create(
    model="gpt-5.6-luna",
    messages=[{"role": "user", "content": "你好"}],
)`,
    curl: `curl ${base}/v1/chat/completions \\
  -H "Authorization: Bearer sk-cp-你的设备Token" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-5.6-luna","messages":[{"role":"user","content":"你好"}]}'`,
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div>
          <h2 className="text-lg font-semibold">系统设置与安全控制中心</h2>
          <p className="text-sm text-muted-foreground">所有修改即时写入 SQLite</p>
        </div>
        {saving && (
          <Badge variant="secondary" className="ml-auto">
            <Loader2 className="h-3 w-3 animate-spin" /> 保存中…
          </Badge>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* IP 白名单 */}
<Card className="min-w-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><ShieldCheck className="h-4 w-4 text-primary" /> IP 白名单与访问控制</CardTitle>
            <CardDescription>开启后仅白名单 IP 可访问网关</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex items-center justify-between rounded-lg border px-3.5 py-3">
              <div>
                <div className="text-sm font-medium">启用 IP 白名单</div>
                <div className="text-xs text-muted-foreground">公网部署建议开启</div>
              </div>
              <Switch
                checked={!!cfg?.ip_whitelist_enabled}
                onCheckedChange={(v) => void patch({ ip_whitelist_enabled: v })}
              />
            </div>
            <div className="flex flex-wrap gap-1.5">
              {ips.map((ip) => (
                <Badge key={ip} variant="secondary" className="gap-1 py-1 font-mono">
                  {ip}
                  <button
                    onClick={() => void patch({ ip_whitelist: ips.filter((x) => x !== ip) })}
                    className="text-muted-foreground hover:text-destructive"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </Badge>
              ))}
              {ips.length === 0 && <span className="text-xs text-muted-foreground">暂无白名单条目</span>}
            </div>
            <div className="flex gap-2">
              <Input placeholder="192.168.1.0/24 或 1.2.3.4" value={ipDraft} onChange={(e) => setIpDraft(e.target.value)} />
              <Button
                variant="outline"
                onClick={() => {
                  const v = ipDraft.trim();
                  if (!v) return toast("请输入 IP", "err");
                  if (ips.includes(v)) return toast("已存在", "warn");
                  void patch({ ip_whitelist: [...ips, v] });
                  setIpDraft("");
                  toast("已添加 " + v);
                }}
              >
                <Plus className="h-3.5 w-3.5" /> 添加
              </Button>
            </div>
          </CardContent>
        </Card>

        {/* 上游密钥状态 */}
<Card className="min-w-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><RotateCcw className="h-4 w-4 text-primary" /> 上游密钥状态</CardTitle>
            <CardDescription>来自 Providers 表的真实记录</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="grid grid-cols-3 gap-3">
              {[
                { k: "已配置", v: upstreams.filter((u) => u.has_key).length, cls: "border-emerald-500/30 bg-emerald-500/5 text-emerald-600" },
                { k: "未配置", v: upstreams.filter((u) => !u.has_key).length, cls: "border-amber-500/30 bg-amber-500/5 text-amber-600" },
                { k: "异常", v: upstreams.filter((u) => u.status === "error").length, cls: "border-red-500/30 bg-red-500/5 text-red-600" },
              ].map((s) => (
                <div key={s.k} className={`rounded-lg border px-3 py-3 ${s.cls}`}>
                  <div className="text-xs opacity-80">{s.k}</div>
                  <div className="mt-0.5 text-2xl font-semibold tabular-nums">{s.v}</div>
                </div>
              ))}
            </div>
            <div className="space-y-1.5 pt-1">
              {upstreams.map((u) => (
                <div key={u.id} className="flex items-center gap-2 text-xs">
                  <span className="max-w-[220px] truncate font-mono">{u.name}</span>
                  <span className="ml-auto font-mono text-muted-foreground">{u.api_key_masked || "—"}</span>
                </div>
              ))}
              {upstreams.length === 0 && <div className="text-xs text-muted-foreground">还没有配置上游</div>}
            </div>
          </CardContent>
        </Card>

        {/* 审计日志 */}
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-center gap-2 space-y-0">
            <div>
              <CardTitle className="flex items-center gap-1.5"><History className="h-4 w-4 text-primary" /> 管理操作审计日志</CardTitle>
              <CardDescription>来自 audit 表</CardDescription>
            </div>
            <Button variant="outline" size="sm" className="ml-auto" onClick={() => void load()}>刷新</Button>
          </CardHeader>
          <CardContent className="overflow-x-auto p-0 pb-1">
            <Table className="min-w-[640px]">
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-4">时间</TableHead>
                  <TableHead>操作人</TableHead>
                  <TableHead>动作</TableHead>
                  <TableHead className="pr-4">结果</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {audit.map((a) => (
                  <TableRow key={a.id}>
                    <TableCell className="pl-4 font-mono text-xs text-muted-foreground">{a.created_at}</TableCell>
                    <TableCell className="text-sm">{a.actor}</TableCell>
                    <TableCell className="text-sm">
                      {a.action}
                      {a.target && <span className="ml-2 font-mono text-xs text-muted-foreground">{a.target}</span>}
                    </TableCell>
                    <TableCell className="pr-4">
                      <Badge variant={a.ok ? "success" : "destructive"}>{a.ok ? "成功" : "失败"}</Badge>
                    </TableCell>
                  </TableRow>
                ))}
                {audit.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={4} className="py-8 text-center text-sm text-muted-foreground">暂无审计记录</TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        {/* OpenAPI */}
<Card className="min-w-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><FileJson className="h-4 w-4 text-primary" /> 接口文档</CardTitle>
            <CardDescription>网关暴露的 OpenAI 兼容接口</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            <Button
              variant="outline" size="sm"
              onClick={async () => {
                const spec = {
                  openapi: "3.0.0",
                  info: { title: "liapi", version: "1.0.0" },
                  servers: [{ url: `${base}/v1` }],
                  paths: {
                    "/chat/completions": { post: { summary: "对话补全", responses: { "200": { description: "OK" } } } },
                    "/models": { get: { summary: "模型列表", responses: { "200": { description: "OK" } } } },
                  },
                };
                const url = URL.createObjectURL(new Blob([JSON.stringify(spec, null, 2)], { type: "application/json" }));
                window.open(url, "_blank");
              }}
            >
              <ExternalLink className="h-3.5 w-3.5" /> 查看 openapi.json
            </Button>
            <Button variant="outline" size="sm" onClick={async () => { await navigator.clipboard.writeText(`${base}/v1`); toast("链接已复制"); }}>
              <Copy className="h-3.5 w-3.5" /> 复制链接
            </Button>
          </CardContent>
        </Card>

        {/* 预算熔断 */}
<Card className="min-w-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><Wallet className="h-4 w-4 text-primary" /> 月度预算与熔断</CardTitle>
            <CardDescription>达到阈值后按策略处理</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label>月度预算 (USD)</Label>
                <Input
                  type="number"
                  defaultValue={cfg?.monthly_budget ?? 20}
                  onBlur={(e) => void patch({ monthly_budget: Number(e.target.value) || 0 })}
                />
              </div>
              <div className="space-y-1.5">
                <Label>超额动作</Label>
                <Select value={cfg?.budget_action ?? "notify"} onValueChange={(v) => void patch({ budget_action: v })}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="notify">仅告警通知</SelectItem>
                    <SelectItem value="throttle">降速限流</SelectItem>
                    <SelectItem value="stop">直接熔断</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between text-sm">
                <Label>熔断阈值</Label>
                <span className="rounded bg-secondary px-1.5 py-0.5 font-mono text-xs tabular-nums">
                  {cfg?.budget_threshold ?? 80}%
                </span>
              </div>
              <Slider
                min={50} max={100} step={5}
                value={[Number(cfg?.budget_threshold ?? 80)]}
                onValueChange={([v]) => setCfg((c) => ({ ...(c ?? {}), budget_threshold: v }))}
                onValueCommit={([v]) => void patch({ budget_threshold: v })}
              />
            </div>
          </CardContent>
        </Card>

        {/* 缓存 */}
<Card className="min-w-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><Database className="h-4 w-4 text-primary" /> 响应缓存</CardTitle>
            <CardDescription>相同请求命中缓存可降低上游费用</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex items-center justify-between rounded-lg border px-3.5 py-3">
              <div>
                <div className="text-sm font-medium">启用响应缓存</div>
                <div className="text-xs text-muted-foreground">按请求体哈希缓存上游响应</div>
              </div>
              <Switch checked={!!cfg?.cache_enabled} onCheckedChange={(v) => void patch({ cache_enabled: v })} />
            </div>
            <div className="space-y-1.5">
              <Label>缓存有效期 TTL（秒）</Label>
              <Input
                type="number"
                defaultValue={cfg?.cache_ttl ?? 300}
                onBlur={(e) => void patch({ cache_ttl: Number(e.target.value) || 0 })}
              />
            </div>
            <div className="flex items-center justify-between rounded-lg bg-secondary/60 px-3.5 py-2.5">
              <span className="text-xs text-muted-foreground">清理网关缓存文件</span>
              <Button
                size="sm" variant="outline"
                onClick={async () => { await api.clearCache(); toast("缓存已清空", "warn"); await load(); }}
              >
                <Trash2 className="h-3.5 w-3.5" /> 一键清空
              </Button>
            </div>
          </CardContent>
        </Card>

        {/* 接入指引 */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><Globe className="h-4 w-4 text-primary" /> 客户端接入指南</CardTitle>
            <CardDescription>把 Base URL 换成网关地址即可</CardDescription>
          </CardHeader>
          <CardContent>
            <Tabs defaultValue="python">
              <TabsList>
                <TabsTrigger value="python">Python</TabsTrigger>
                <TabsTrigger value="curl">cURL</TabsTrigger>
              </TabsList>
              {(["python", "curl"] as const).map((k) => (
                <TabsContent key={k} value={k}>
                  <div className="relative">
                    <Button
                      variant="ghost" size="sm"
                      className="absolute right-2 top-2 h-7 text-zinc-300 hover:bg-white/10 hover:text-white"
                      onClick={async () => { await navigator.clipboard.writeText(CODE[k]); toast("已复制"); }}
                    >
                      <Copy className="h-3.5 w-3.5" /> 复制
                    </Button>
                    <pre className="overflow-x-auto rounded-lg bg-zinc-950 p-4 pt-11 font-mono text-[11.5px] leading-relaxed text-zinc-100">
                      <code>{CODE[k]}</code>
                    </pre>
                  </div>
                </TabsContent>
              ))}
            </Tabs>
          </CardContent>
        </Card>

        {/* Webhook */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="flex items-center gap-1.5"><Webhook className="h-4 w-4 text-primary" /> Webhook 告警</CardTitle>
            <CardDescription>发送真实 HTTP POST 到你的地址</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex flex-wrap gap-1.5">
              {["上游故障", "额度超限", "异常登录", "密钥到期"].map((e) => (
                <Badge key={e} variant="secondary">{e}</Badge>
              ))}
            </div>
            <div className="flex flex-col gap-2 sm:flex-row">
              <Input
                className="font-mono text-xs"
                placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/xxxx"
                value={hook}
                onChange={(e) => setHook(e.target.value)}
              />
              <div className="flex gap-2">
                <Button variant="outline" onClick={() => void patch({ webhook_url: hook }).then(() => toast("已保存"))}>
                  <Save className="h-3.5 w-3.5" /> 保存
                </Button>
                <Button
                  variant="outline"
                  onClick={async () => {
                    try {
                      const r = await api.testWebhook(hook);
                      toast(r.ok ? "测试事件已发送" : `接收端返回 HTTP ${r.http_status}`, r.ok ? "ok" : "warn");
                      await load();
                    } catch (e: any) {
                      toast(e.message, "err");
                    }
                  }}
                >
                  <Send className="h-3.5 w-3.5" /> 测试发送
                </Button>
              </div>
            </div>
            {!hook && (
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <AlertTriangle className="h-3 w-3" /> 填写地址后才能测试发送
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
