import { useEffect, useRef, useState } from "react";
import { Bot, Download, KeyRound, Maximize2, Minimize2, Send, Sparkles, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input, Textarea } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { useToast } from "@/context/theme";
import { api, type Device, type ModelRow } from "@/lib/api";

type Msg = { id: number; role: "user" | "assistant"; content: string; meta?: { latencyMs: number; tokens: number } };

const TEMPLATES = [
  { emoji: "🛠️", label: "代码重构与类型优化", prompt: "请重构下面这段 TypeScript，消除 any 并补全类型：\n\nfunction handle(data: any) { return data.items.map((x: any) => x.value); }" },
  { emoji: "🔵", label: "专业中英双向翻译", prompt: "请把下面这句话翻译成地道的英文，保持技术文档语气：\n「网关会在毫秒级完成协议适配与故障降级。」" },
  { emoji: "📊", label: "结构化 JSON 提取", prompt: "从下面文本中抽取字段并只输出 JSON：订单号 A2041，金额 $19.90，状态已发货。\n要求字段：order_id, amount, status" },
  { emoji: "🧠", label: "逐步逻辑推理 (Chain of Thought)", prompt: "一个水池有进水管 4 小时注满，出水管 6 小时放空，两管齐开多久注满？请给出逐步推理。" },
];

/** 调用真实网关：POST /v1/chat/completions */
async function callGateway(
  token: string,
  body: Record<string, any>,
  onDelta?: (chunk: string) => void
): Promise<{ content: string; inTokens: number; outTokens: number }> {
  const res = await fetch("/v1/chat/completions", {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify(body),
  });
  const text = await res.text();
  if (!res.ok) {
    let msg = text;
    try {
      msg = JSON.parse(text)?.error?.message ?? text;
    } catch { /* 原样展示 */ }
    throw new Error(msg || `HTTP ${res.status}`);
  }
  const json = JSON.parse(text);
  const content = json?.choices?.[0]?.message?.content ?? "(上游返回了空内容)";
  if (onDelta) onDelta(content);
  return {
    content,
    inTokens: json?.usage?.prompt_tokens ?? 0,
    outTokens: json?.usage?.completion_tokens ?? 0,
  };
}

export default function Playground() {
  const toast = useToast();
  const [model, setModel] = useState("gpt-5.6-luna");
  const [models, setModels] = useState<string[]>(["gpt-5.6-luna"]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [token, setToken] = useState("");
  const [system, setSystem] = useState("你是一个严谨的网关测试助手，回答简洁。");
  const [expanded, setExpanded] = useState(false);
  const [temp, setTemp] = useState([0.7]);
  const [maxTok, setMaxTok] = useState([2048]);
  const [stream, setStream] = useState(true);
  const [input, setInput] = useState("");
  const [msgs, setMsgs] = useState<Msg[]>([]);
  const [typing, setTyping] = useState(false);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);
  const scroller = useRef<HTMLDivElement>(null);

  // 载入真实模型别名
  useEffect(() => {
    api
      .listModels()
      .then((rows: ModelRow[]) => {
        const aliases = rows.map((m) => m.alias).filter(Boolean);
        if (aliases.length) {
          setModels(aliases);
          setModel((m) => (aliases.includes(m) ? m : aliases[0]));
        }
      })
      .catch(() => {});
  }, []);

  // 载入真实设备，取一个可用 Token 供本地调试
  useEffect(() => {
    api
      .listTokens()
      .then((devs) => {
        setDevices(devs);
        const ok = devs.find((d) => d.status === "normal");
        if (ok) return api.rotateToken(ok.id).then((r) => setToken(r.token ?? ""));
      })
      .catch(() => {});
  }, []);

  useEffect(() => () => { if (timer.current) clearInterval(timer.current); }, []);
  useEffect(() => {
    scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: "smooth" });
  }, [msgs, typing]);

  const send = async (raw?: string) => {
    const text = (raw ?? input).trim();
    if (!text || typing) return;
    if (!token) return toast("没有可用的设备 Token，请先到「设备 Token」页面签发", "err");

    setInput("");
    const uid = Date.now();
    setMsgs((m) => [...m, { id: uid, role: "user", content: text }]);
    setTyping(true);
    const started = performance.now();
    const aid = uid + 1;

    try {
      const messages = [
        ...(system.trim() ? [{ role: "system", content: system }] : []),
        { role: "user", content: text },
      ];
      const r = await callGateway(token, {
        model,
        messages,
        temperature: temp[0],
        max_tokens: maxTok[0],
      });

      const full = r.content;
      setMsgs((m) => [...m, { id: aid, role: "assistant", content: "" }]);
      const latency = Math.round(performance.now() - started);

      if (!stream) {
        setMsgs((m) =>
          m.map((x) =>
            x.id === aid ? { ...x, content: full, meta: { latencyMs: latency, tokens: r.inTokens + r.outTokens } } : x
          )
        );
        setTyping(false);
        return;
      }

      let i = 0;
      await new Promise<void>((resolve) => {
        timer.current = setInterval(() => {
          i += 2;
          setMsgs((m) => m.map((x) => (x.id === aid ? { ...x, content: full.slice(0, i) } : x)));
          if (i >= full.length) {
            if (timer.current) clearInterval(timer.current);
            timer.current = null;
            setMsgs((m) =>
              m.map((x) =>
                x.id === aid ? { ...x, content: full, meta: { latencyMs: latency, tokens: r.inTokens + r.outTokens } } : x
              )
            );
            resolve();
          }
        }, 18);
      });
      setTyping(false);
    } catch (e: any) {
      setMsgs((m) => [
        ...m,
        {
          id: aid,
          role: "assistant",
          content: `调用失败：${e.message}`,
          meta: { latencyMs: Math.round(performance.now() - started), tokens: 0 },
        },
      ]);
      setTyping(false);
      toast(e.message, "err");
    }
  };

  const clearChat = () => {
    if (timer.current) clearInterval(timer.current);
    setMsgs([]);
    setTyping(false);
    toast("对话已清空", "warn");
  };

  const exportChat = () => {
    if (!msgs.length) return toast("暂无对话可导出", "warn");
    const blob = new Blob(
      [JSON.stringify({ model, system, temperature: temp[0], max_tokens: maxTok[0], messages: msgs }, null, 2)],
      { type: "application/json" }
    );
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `playground-${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(a.href);
    toast("对话已导出为 JSON");
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <h2 className="text-lg font-semibold">API Playground (在线调试与对话工作台)</h2>
          <p className="text-sm text-muted-foreground">
            真实调用本地网关 /v1/chat/completions
            {token ? <span className="ml-1 font-mono text-xs">（{token.slice(0, 12)}…）</span> : null}
          </p>
        </div>
        <div className="ml-auto flex gap-2">
          <Button variant="outline" size="sm" onClick={exportChat}>
            <Download className="h-3.5 w-3.5" /> 导出对话
          </Button>
          <Button variant="outline" size="sm" onClick={clearChat}>
            <Trash2 className="h-3.5 w-3.5" /> 清空对话
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>模型与运行参数</CardTitle>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-1.5">
                <Label>选择测试模型 / 别名</Label>
                <Select value={model} onValueChange={setModel}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {models.map((m) => (
                      <SelectItem key={m} value={m}>{m}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <Label>System Prompt 系统提示词</Label>
                  <button
                    className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
                    onClick={() => setExpanded((v) => !v)}
                  >
                    {expanded ? <Minimize2 className="h-3 w-3" /> : <Maximize2 className="h-3 w-3" />}
                    {expanded ? "收起编辑" : "展开编辑"}
                  </button>
                </div>
                {expanded ? (
                  <Textarea rows={5} value={system} onChange={(e) => setSystem(e.target.value)} />
                ) : (
                  <Input value={system} onChange={(e) => setSystem(e.target.value)} />
                )}
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between text-sm">
                  <Label>Temperature (多样性)</Label>
                  <span className="rounded bg-secondary px-1.5 py-0.5 font-mono text-xs tabular-nums">{temp[0].toFixed(1)}</span>
                </div>
                <Slider min={0} max={2} step={0.1} value={temp} onValueChange={setTemp} />
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between text-sm">
                  <Label>Max Tokens (最大生成长度)</Label>
                  <span className="rounded bg-secondary px-1.5 py-0.5 font-mono text-xs tabular-nums">{maxTok[0]}</span>
                </div>
                <Slider min={128} max={8192} step={128} value={maxTok} onValueChange={setMaxTok} />
              </div>

              <div className="flex items-center justify-between rounded-lg border px-3.5 py-3">
                <div>
                  <div className="text-sm font-medium">流式响应 (Stream)</div>
                  <div className="text-xs text-muted-foreground">逐字实时打字机渲染</div>
                </div>
                <Switch checked={stream} onCheckedChange={setStream} />
              </div>

              <div className="rounded-lg bg-secondary/60 px-3.5 py-2.5 text-xs text-muted-foreground">
                <KeyRound className="mr-1.5 inline h-3 w-3" />
                当前设备：{devices.find((d) => d.status === "normal")?.name ?? "（无）"} · 共 {devices.length} 台
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>快捷测试模板</CardTitle>
              <CardDescription>点击即发送</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {TEMPLATES.map((t) => (
                <button
                  key={t.label}
                  onClick={() => void send(t.prompt)}
                  disabled={typing}
                  className="flex w-full items-center gap-2.5 rounded-lg bg-secondary px-3.5 py-2.5 text-left text-sm font-medium text-secondary-foreground transition-colors hover:bg-accent disabled:opacity-50"
                >
                  <span aria-hidden>{t.emoji}</span>
                  {t.label}
                </button>
              ))}
            </CardContent>
          </Card>
        </div>

        <Card className="flex min-h-[420px] flex-col overflow-hidden lg:h-[min(70vh,720px)]">
          <div ref={scroller} className="flex-1 overflow-y-auto p-5">
            {msgs.length === 0 ? (
              <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
                <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10">
                  <Sparkles className="h-6 w-6 text-primary" />
                </div>
                <div className="text-sm font-semibold">开始您的模型测试</div>
                <div className="max-w-[300px] text-xs leading-relaxed text-muted-foreground">
                  选择一个模型并在下方输入测试内容，或点击左侧快捷模板立即体验。
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                {msgs.map((m) =>
                  m.role === "user" ? (
                    <div key={m.id} className="flex justify-end">
                      <div className="max-w-[75%] whitespace-pre-wrap rounded-2xl rounded-br-sm bg-primary px-4 py-2.5 text-sm text-primary-foreground shadow-sm">
                        {m.content}
                      </div>
                    </div>
                  ) : (
                    <div key={m.id} className="flex gap-2.5">
                      <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-secondary">
                        <Bot className="h-4 w-4 text-primary" />
                      </div>
                      <div className="max-w-[80%]">
                        <div className="whitespace-pre-wrap rounded-2xl rounded-tl-sm border bg-card px-4 py-2.5 text-sm shadow-sm">
                          {m.content}
                          {!m.meta && <span className="ml-0.5 inline-block h-4 w-[2px] animate-pulse bg-foreground/70 align-middle" />}
                        </div>
                        {m.meta && (
                          <div className="mt-1.5 flex gap-2 text-[11px] text-muted-foreground">
                            <Badge variant="secondary" className="font-mono">{m.meta.latencyMs} ms</Badge>
                            <Badge variant="secondary" className="font-mono">{m.meta.tokens} tokens</Badge>
                          </div>
                        )}
                      </div>
                    </div>
                  )
                )}
                {typing && !msgs.some((m) => m.role === "assistant" && !m.meta) && (
                  <div className="flex items-center gap-2 pl-9 text-xs text-muted-foreground">
                    <span className="h-1.5 w-1.5 animate-ping rounded-full bg-primary" /> {model} 正在思考…
                  </div>
                )}
              </div>
            )}
          </div>

          <div className="border-t bg-card p-3">
            <div className="flex items-end gap-2">
              <Textarea
                rows={1}
                className="max-h-32 min-h-9 flex-1 resize-none"
                placeholder={`向 ${model} 发送消息 (Enter 发送, Shift+Enter 换行)...`}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey) {
                    e.preventDefault();
                    void send();
                  }
                }}
              />
              <Button onClick={() => void send()} disabled={typing || !input.trim()}>
                <Send className="h-4 w-4" /> 发送
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
}
