import { useMemo, useState } from "react";
import {
  Activity, AlertTriangle, Ban, CheckCircle2, Loader2, Pencil, Plus, Radar, RefreshCw, Search, Trash2,
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
import { api, type Upstream } from "@/lib/api";
import { useAsync } from "@/lib/useAsync";

function healthCell(u: Upstream, probing: string | null, latency: Record<string, number>) {
  if (probing === u.id)
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
        <Loader2 className="h-3 w-3 animate-spin" /> 探测中…
      </span>
    );
  if (!u.enabled) return <Badge variant="secondary">已停用</Badge>;
  if (u.status === "error") return <Badge variant="destructive">异常</Badge>;
  const ms = latency[u.id];
  return (
    <span className="inline-flex items-center gap-1.5">
      <Badge variant="success">正常</Badge>
      {ms != null && <span className="font-mono text-xs text-muted-foreground">{ms} ms</span>}
    </span>
  );
}

export default function Providers() {
  const toast = useToast();
  const { data, loading, error, reload } = useAsync<Upstream[]>(() => api.listUpstreams(), []);
  const list = data ?? [];

  const [q, setQ] = useState("");
  const [proto, setProto] = useState("all");
  const [probing, setProbing] = useState<string | null>(null);
  const [latency, setLatency] = useState<Record<string, number>>({});
  const [busy, setBusy] = useState(false);

  const [adding, setAdding] = useState(false);
  const [form, setForm] = useState({ name: "", protocol: "OpenAI", key: "" });

  const [editing, setEditing] = useState<Upstream | null>(null);
  const [editForm, setEditForm] = useState({ name: "", protocol: "OpenAI", key: "" });

  const [deleting, setDeleting] = useState<Upstream | null>(null);

  const filtered = useMemo(
    () =>
      list.filter(
        (p) =>
          (proto === "all" || p.protocol === proto) && p.name.toLowerCase().includes(q.trim().toLowerCase())
      ),
    [list, q, proto]
  );

  const probe = async (u: Upstream) => {
    setProbing(u.id);
    try {
      const r = await api.probeUpstream(u.id);
      setLatency((l) => ({ ...l, [u.id]: r.latency_ms }));
      await reload();
      toast(
        r.status === "normal" ? `探测成功 · ${r.latency_ms} ms` : `探测失败 · ${r.error ?? "HTTP " + r.http_status}`,
        r.status === "normal" ? "ok" : "err"
      );
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setProbing(null);
    }
  };

  const probeAll = async () => {
    if (!list.length) return;
    toast("开始探测全部服务商健康…", "warn");
    for (const u of list) await probe(u);
  };

  const addProvider = async () => {
    if (!form.name.trim()) return toast("请填写上游地址", "err");
    setBusy(true);
    try {
      await api.createUpstream({ name: form.name.trim(), protocol: form.protocol, api_key: form.key });
      setAdding(false);
      setForm({ name: "", protocol: "OpenAI", key: "" });
      await reload();
      toast("Provider 已新增");
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
      // 密钥留空 = 保留原值
      const patch: Record<string, any> = { name: editForm.name.trim(), protocol: editForm.protocol };
      if (editForm.key.trim()) patch.api_key = editForm.key.trim();
      await api.updateUpstream(editing.id, patch);
      setEditing(null);
      await reload();
      toast("已保存");
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setBusy(false);
    }
  };

  const toggle = async (u: Upstream) => {
    try {
      await api.updateUpstream(u.id, { enabled: !u.enabled });
      await reload();
      toast(u.enabled ? "已停用该上游" : "已启用该上游", "warn");
    } catch (e: any) {
      toast(e.message, "err");
    }
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    setBusy(true);
    try {
      await api.deleteUpstream(deleting.id);
      setDeleting(null);
      await reload();
      toast("已删除", "warn");
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索上游地址…" className="w-full pl-8 sm:w-64" />
        </div>
        <Select value={proto} onValueChange={setProto}>
          <SelectTrigger className="w-36"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部协议</SelectItem>
            <SelectItem value="OpenAI">OpenAI</SelectItem>
            <SelectItem value="Anthropic">Anthropic</SelectItem>
          </SelectContent>
        </Select>
        <div className="ml-auto flex gap-2">
          <Button variant="outline" size="sm" onClick={() => void reload()} disabled={loading}>
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </Button>
          <Button variant="outline" size="sm" onClick={() => void probeAll()}>
            <Radar className="h-3.5 w-3.5" /> 探测健康
          </Button>
          <Button size="sm" onClick={() => setAdding(true)}>
            <Plus className="h-3.5 w-3.5" /> 新增 Provider
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-red-500/40">
          <CardContent className="flex items-center gap-3 p-4">
            <AlertTriangle className="h-4 w-4 shrink-0 text-red-500" />
            <div className="text-sm">
              <div className="font-medium">无法连接后端</div>
              <div className="text-muted-foreground">{error}</div>
            </div>
            <Button size="sm" variant="outline" className="ml-auto" onClick={() => void reload()}>重试</Button>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">名称</TableHead>
                <TableHead>协议类型</TableHead>
                <TableHead>健康状态</TableHead>
                <TableHead className="pr-4 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && list.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="py-10 text-center text-sm text-muted-foreground">
                    <Loader2 className="mx-auto mb-2 h-4 w-4 animate-spin" /> 正在加载…
                  </TableCell>
                </TableRow>
              )}
              {filtered.map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="max-w-[320px] truncate pl-4 font-mono text-xs">{p.name}</TableCell>
                  <TableCell>
                    <Badge variant={p.protocol === "OpenAI" ? "default" : "violet"}>{p.protocol}</Badge>
                  </TableCell>
                  <TableCell>{healthCell(p, probing, latency)}</TableCell>
                  <TableCell className="pr-4 text-right">
                    <div className="inline-flex gap-1">
                      <Button variant="ghost" size="icon" className="h-7 w-7" title="探测" onClick={() => void probe(p)}>
                        <Activity className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost" size="icon" className="h-7 w-7"
                        title={p.enabled ? "停用" : "启用"}
                        onClick={() => void toggle(p)}
                      >
                        {p.enabled ? <Ban className="h-3.5 w-3.5" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
                      </Button>
                      <Button
                        variant="ghost" size="icon" className="h-7 w-7" title="编辑"
                        onClick={() => { setEditing(p); setEditForm({ name: p.name, protocol: p.protocol, key: "" }); }}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive" title="删除" onClick={() => setDeleting(p)}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {!loading && filtered.length === 0 && !error && (
                <TableRow>
                  <TableCell colSpan={4} className="py-10 text-center text-sm text-muted-foreground">
                    没有匹配的 Provider
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
            <DialogTitle>新增 Provider</DialogTitle>
            <DialogDescription>接入一个 OpenAI / Anthropic 兼容的上游服务</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>上游 Base URL</Label>
              <Input placeholder="https://api.example.com/v1" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </div>
            <div className="space-y-1.5">
              <Label>协议类型</Label>
              <Select value={form.protocol} onValueChange={(v) => setForm({ ...form, protocol: v })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="OpenAI">OpenAI</SelectItem>
                  <SelectItem value="Anthropic">Anthropic</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>API Key</Label>
              <Input type="password" placeholder="sk-…" value={form.key} onChange={(e) => setForm({ ...form, key: e.target.value })} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAdding(false)}>取消</Button>
            <Button onClick={() => void addProvider()} disabled={busy}>
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑 */}
      <Dialog open={!!editing} onOpenChange={(v) => !v && setEditing(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑 Provider</DialogTitle>
            <DialogDescription>密钥留空表示保持原值不变</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>上游 Base URL</Label>
              <Input value={editForm.name} onChange={(e) => setEditForm({ ...editForm, name: e.target.value })} />
            </div>
            <div className="space-y-1.5">
              <Label>协议类型</Label>
              <Select value={editForm.protocol} onValueChange={(v) => setEditForm({ ...editForm, protocol: v })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="OpenAI">OpenAI</SelectItem>
                  <SelectItem value="Anthropic">Anthropic</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>API Key</Label>
              <Input
                type="password"
                placeholder={editing?.has_key ? `当前：${editing.api_key_masked}（留空不改）` : "尚未配置"}
                value={editForm.key}
                onChange={(e) => setEditForm({ ...editForm, key: e.target.value })}
              />
            </div>
          </div>
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
              <AlertTriangle className="h-4 w-4 text-red-500" /> 确认删除 Provider？
            </DialogTitle>
            <DialogDescription>
              <span className="font-medium text-foreground">{deleting?.name}</span> 将被移除，绑定到它的模型会失去上游。
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
