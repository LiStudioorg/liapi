import { useMemo, useState } from "react";
import {
  AlertTriangle, ChevronRight, Loader2, Pencil, Plus, RefreshCw, Search, Trash2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useToast } from "@/context/theme";
import { api, type ModelRow, type Upstream } from "@/lib/api";
import { useAsync } from "@/lib/useAsync";
import { cn } from "@/lib/utils";

const CAPS = ["全部模型", "视觉", "工具调用", "深度思考", "128K+上下文"] as const;
const ALL_CAPS = ["视觉", "工具调用", "深度思考", "128K+上下文"];

const STEPS = [
  { n: 1, title: "请求 Model", desc: "客户端携带 model / 别名发起请求" },
  { n: 2, title: "别名优先", desc: "命中别名表则改写为上游真实模型" },
  { n: 3, title: "原始直通", desc: "未命中时按原名透传给上游" },
  { n: 4, title: "故障降级", desc: "上游异常自动切换备选线路" },
];

const emptyForm = { upstream: "", display: "", alias: "", caps: [] as string[], upstream_id: "none" };

export default function Models() {
  const toast = useToast();
  const models = useAsync<ModelRow[]>(() => api.listModels(), []);
  const ups = useAsync<Upstream[]>(() => api.listUpstreams(), []);
  const rows = models.data ?? [];
  const upstreams = ups.data ?? [];

  const [q, setQ] = useState("");
  const [prov, setProv] = useState("all");
  const [cap, setCap] = useState<(typeof CAPS)[number]>("全部模型");
  const [busy, setBusy] = useState(false);

  const [adding, setAdding] = useState(false);
  const [form, setForm] = useState({ ...emptyForm });
  const [editing, setEditing] = useState<ModelRow | null>(null);
  const [deleting, setDeleting] = useState<ModelRow | null>(null);

  const filtered = useMemo(
    () =>
      rows.filter(
        (m) =>
          (prov === "all" || m.upstream_id === prov) &&
          (cap === "全部模型" || m.caps.includes(cap)) &&
          (m.upstream + m.display + m.alias).toLowerCase().includes(q.trim().toLowerCase())
      ),
    [rows, q, prov, cap]
  );

  const reloadAll = async () => {
    await Promise.all([models.reload(), ups.reload()]);
  };

  const create = async () => {
    if (!form.upstream.trim()) return toast("请填写上游模型名", "err");
    setBusy(true);
    try {
      await api.createModel({
        upstream: form.upstream.trim(),
        display: (form.display || form.upstream).trim(),
        alias: (form.alias || form.upstream).trim(),
        caps: form.caps,
        upstream_id: form.upstream_id === "none" ? null : form.upstream_id,
      });
      setAdding(false);
      setForm({ ...emptyForm });
      await models.reload();
      toast("模型已新增");
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setBusy(false);
    }
  };

  const saveEdit = async () => {
    if (!editing) return;
    setBusy(true);
    try {
      await api.updateModel(editing.id, {
        upstream: form.upstream.trim(),
        display: form.display.trim(),
        alias: form.alias.trim(),
        caps: form.caps,
        upstream_id: form.upstream_id === "none" ? null : form.upstream_id,
      });
      setEditing(null);
      await models.reload();
      toast("已保存");
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setBusy(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    setBusy(true);
    try {
      await api.deleteModel(deleting.id);
      setDeleting(null);
      await models.reload();
      toast("已删除", "warn");
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setBusy(false);
    }
  };

  const openEdit = (m: ModelRow) => {
    setEditing(m);
    setForm({
      upstream: m.upstream,
      display: m.display,
      alias: m.alias,
      caps: [...m.caps],
      upstream_id: m.upstream_id ?? "none",
    });
  };

  /** 新增/编辑共用的表单 —— 用函数而非复用的 JSX 元素，确保两个 Dialog 各有独立实例 */
  const renderForm = () => (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label>上游模型名</Label>
        <Input placeholder="gpt-4o-mini" value={form.upstream} onChange={(e) => setForm({ ...form, upstream: e.target.value })} />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label>显示名称</Label>
          <Input placeholder="GPT-4o mini" value={form.display} onChange={(e) => setForm({ ...form, display: e.target.value })} />
        </div>
        <div className="space-y-1.5">
          <Label>客户端别名</Label>
          <Input placeholder="gpt-5.6-luna" value={form.alias} onChange={(e) => setForm({ ...form, alias: e.target.value })} />
        </div>
      </div>
      <div className="space-y-1.5">
        <Label>绑定 Provider</Label>
        <Select value={form.upstream_id} onValueChange={(v) => setForm({ ...form, upstream_id: v })}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="none">不绑定</SelectItem>
            {upstreams.map((u) => (
              <SelectItem key={u.id} value={u.id}>{u.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label>能力标签</Label>
        <div className="flex flex-wrap gap-1.5">
          {ALL_CAPS.map((c) => {
            const on = form.caps.includes(c);
            return (
              <button
                key={c}
                type="button"
                onClick={() =>
                  setForm({ ...form, caps: on ? form.caps.filter((x) => x !== c) : [...form.caps, c] })
                }
                className={cn(
                  "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
                  on ? "border-primary bg-primary/10 text-primary" : "text-muted-foreground hover:bg-accent"
                )}
              >
                {c}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );

  return (
    <div className="space-y-4">
      {/* 路由逻辑 */}
      <Card>
        <CardContent className="p-4">
          <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
            <span className="shrink-0 text-sm font-semibold">路由逻辑</span>
            {/* 中等宽度先并排 2×2，超宽才 4 个横排，绝不把 4 个挤进一行 */}
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-4">
              {STEPS.map((s, i) => (
                <div key={s.n} className="flex items-center gap-2">
                  <div className="flex min-w-0 flex-1 items-center gap-2.5 rounded-lg bg-secondary px-3 py-2">
                    <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-primary-foreground">
                      {s.n}
                    </span>
                    <div className="min-w-0">
                      <div className="truncate text-xs font-semibold leading-tight">{s.title}</div>
                      <div className="truncate text-[11px] text-muted-foreground">{s.desc}</div>
                    </div>
                  </div>
                  {i < STEPS.length - 1 && (
                    <ChevronRight className="hidden h-4 w-4 shrink-0 text-muted-foreground xl:block" />
                  )}
                </div>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* 筛选 */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索模型 / 别名…" className="w-full pl-8 sm:w-60" />
        </div>
        <Select value={prov} onValueChange={setProv}>
          <SelectTrigger className="w-full sm:w-56"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部 Provider</SelectItem>
            {upstreams.map((u) => (
              <SelectItem key={u.id} value={u.id}>{u.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="flex flex-wrap gap-1.5">
          {CAPS.map((c) => (
            <button
              key={c}
              onClick={() => setCap(c)}
              className={cn(
                "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
                cap === c ? "border-primary bg-primary/10 text-primary" : "text-muted-foreground hover:bg-accent"
              )}
            >
              {c}
            </button>
          ))}
        </div>
        <div className="ml-auto flex gap-2">
          <Button variant="outline" size="sm" onClick={() => void reloadAll()} disabled={models.loading}>
            <RefreshCw className={`h-3.5 w-3.5 ${models.loading ? "animate-spin" : ""}`} />
          </Button>
          <Button size="sm" onClick={() => { setForm({ ...emptyForm, upstream_id: upstreams[0]?.id ?? "none" }); setAdding(true); }}>
            <Plus className="h-3.5 w-3.5" /> 新增模型
          </Button>
        </div>
      </div>

      {models.error && (
        <Card className="border-red-500/40">
          <CardContent className="flex items-center gap-3 p-4">
            <AlertTriangle className="h-4 w-4 shrink-0 text-red-500" />
            <div className="text-sm text-muted-foreground">{models.error}</div>
            <Button size="sm" variant="outline" className="ml-auto" onClick={() => void models.reload()}>重试</Button>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">上游模型名 / 能力</TableHead>
                <TableHead>显示名称</TableHead>
                <TableHead>客户端别名</TableHead>
                <TableHead>绑定 Provider</TableHead>
                <TableHead className="pr-4 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {models.loading && rows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                    <Loader2 className="mx-auto mb-2 h-4 w-4 animate-spin" /> 正在加载…
                  </TableCell>
                </TableRow>
              )}
              {filtered.map((m) => (
                <TableRow key={m.id}>
                  <TableCell className="pl-4">
                    <div className="font-mono text-xs font-medium">{m.upstream}</div>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {m.caps.map((c) => (
                        <Badge key={c} variant="secondary" className="text-[10px]">{c}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="text-sm">{m.display}</TableCell>
                  <TableCell>
                    <code className="rounded bg-secondary px-1.5 py-0.5 font-mono text-xs">{m.alias}</code>
                  </TableCell>
                  <TableCell className="max-w-[200px] truncate font-mono text-xs text-muted-foreground">
                    {m.upstream_name ?? "—"}
                  </TableCell>
                  <TableCell className="pr-4 text-right">
                    <div className="inline-flex gap-1">
                      <Button variant="ghost" size="icon" className="h-7 w-7" title="编辑" onClick={() => openEdit(m)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost" size="icon" className="h-7 w-7 text-destructive" title="删除"
                        onClick={() => setDeleting(m)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {!models.loading && filtered.length === 0 && !models.error && (
                <TableRow>
                  <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                    没有匹配的模型
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* 新增 */}
      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新增模型</DialogTitle>
            <DialogDescription>把上游模型名映射成一个客户端别名</DialogDescription>
          </DialogHeader>
          {renderForm()}
          <DialogFooter>
            <Button variant="outline" onClick={() => setAdding(false)}>取消</Button>
            <Button onClick={() => void create()} disabled={busy}>
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑 */}
      <Dialog open={!!editing} onOpenChange={(v) => !v && setEditing(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑模型</DialogTitle>
            <DialogDescription>{editing?.id}</DialogDescription>
          </DialogHeader>
          {renderForm()}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)}>取消</Button>
            <Button onClick={() => void saveEdit()} disabled={busy}>
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 删除确认 */}
      <Dialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-red-500" /> 确认删除模型？
            </DialogTitle>
            <DialogDescription>
              别名 <span className="font-mono text-foreground">{deleting?.alias}</span> 将被移除，客户端再调用它会按原名透传。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>取消</Button>
            <Button variant="destructive" onClick={() => void confirmDelete()} disabled={busy}>
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
