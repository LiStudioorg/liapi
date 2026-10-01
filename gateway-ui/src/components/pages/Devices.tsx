import { useState } from "react";
import {
  AlertTriangle, Ban, CheckCircle2, Copy, KeyRound, Loader2, Pencil, Plus, RefreshCw, Trash2,
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
import { api, type Device } from "@/lib/api";
import { useAsync } from "@/lib/useAsync";

/** RPM 下拉的候选值 */
const RPM_OPTIONS = [
  { value: "none", label: "无限制" },
  { value: "30", label: "30 / min" },
  { value: "60", label: "60 / min" },
  { value: "300", label: "300 / min" },
];

export default function Devices() {
  const toast = useToast();
  const { data, loading, error, reload } = useAsync<Device[]>(() => api.listTokens(), []);
  const devices = data ?? [];

  // 新建
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [newRpm, setNewRpm] = useState("none");
  const [issued, setIssued] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // 编辑
  const [editing, setEditing] = useState<Device | null>(null);
  const [editName, setEditName] = useState("");
  const [editRpm, setEditRpm] = useState("none");

  // 删除确认
  const [deleting, setDeleting] = useState<Device | null>(null);

  const openEdit = (d: Device) => {
    setEditing(d);
    setEditName(d.name);
    setEditRpm(d.rpm === null ? "none" : String(d.rpm));
  };

  const create = async () => {
    if (!newName.trim()) return toast("请填写设备名称", "err");
    setBusy(true);
    try {
      const d = await api.createToken(newName.trim(), newRpm === "none" ? null : Number(newRpm));
      setIssued(d.token ?? null);
      setNewName("");
      await reload();
      toast("设备 Token 已签发");
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setBusy(false);
    }
  };

  const saveEdit = async () => {
    if (!editing) return;
    if (!editName.trim()) return toast("设备名称不能为空", "err");
    setBusy(true);
    try {
      await api.updateToken(editing.id, {
        name: editName.trim(),
        rpm: editRpm === "none" ? null : Number(editRpm),
      });
      setEditing(null);
      await reload();
      toast("已保存修改");
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setBusy(false);
    }
  };

  const toggleStatus = async (d: Device) => {
    try {
      await api.updateToken(d.id, { status: d.status === "normal" ? "disabled" : "normal" });
      await reload();
      toast(d.status === "normal" ? "已禁用该设备" : "已启用该设备", "warn");
    } catch (e: any) {
      toast(e.message, "err");
    }
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    setBusy(true);
    try {
      await api.deleteToken(deleting.id);
      setDeleting(null);
      await reload();
      toast("设备已删除", "warn");
    } catch (e: any) {
      toast(e.message, "err");
    } finally {
      setBusy(false);
    }
  };

  const rotate = async (d: Device) => {
    try {
      const r = await api.rotateToken(d.id);
      setIssued(r.token ?? null);
      setCreating(true);
      await reload();
      toast("已重新签发密钥");
    } catch (e: any) {
      toast(e.message, "err");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <p className="text-sm text-muted-foreground">
          为手机 / CLI / IoT 设备签发独立 Token，可分别限流与审计
        </p>
        <div className="ml-auto flex gap-2">
          <Button variant="outline" size="sm" onClick={() => void reload()} disabled={loading}>
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} /> 刷新
          </Button>
          <Button size="sm" onClick={() => { setIssued(null); setCreating(true); }}>
            <Plus className="h-3.5 w-3.5" /> 签发新设备 Token
          </Button>
        </div>
      </div>

      {/* 加载 / 错误 */}
      {error && (
        <Card className="border-red-500/40">
          <CardContent className="flex items-center gap-3 p-4">
            <AlertTriangle className="h-4 w-4 shrink-0 text-red-500" />
            <div className="text-sm">
              <div className="font-medium">无法连接后端</div>
              <div className="text-muted-foreground">{error} —— 请确认已运行 npm run server</div>
            </div>
            <Button size="sm" variant="outline" className="ml-auto" onClick={() => void reload()}>
              重试
            </Button>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">设备名称</TableHead>
                <TableHead>ID</TableHead>
                <TableHead>当前状态</TableHead>
                <TableHead>每分钟限流 (RPM)</TableHead>
                <TableHead className="text-right">本月消费</TableHead>
                <TableHead className="pr-4 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && devices.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                    <Loader2 className="mx-auto mb-2 h-4 w-4 animate-spin" /> 正在加载…
                  </TableCell>
                </TableRow>
              )}

              {devices.map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="pl-4 text-sm font-medium">{d.name}</TableCell>
                  <TableCell>
                    <code className="rounded bg-secondary px-1.5 py-0.5 font-mono text-xs">{d.id}</code>
                  </TableCell>
                  <TableCell>
                    {d.status === "normal" ? (
                      <Badge variant="success">
                        <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> 正常
                      </Badge>
                    ) : (
                      <Badge variant="secondary">已停用</Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {d.rpm ?? "无限制"}
                    {d.rpm ? " / min" : ""}
                  </TableCell>
                  <TableCell className="text-right font-mono text-sm tabular-nums">
                    ${d.month_cost.toFixed(2)}
                  </TableCell>
                  <TableCell className="pr-4 text-right">
                    <div className="inline-flex gap-1">
                      <Button variant="ghost" size="icon" className="h-7 w-7" title="编辑" onClick={() => openEdit(d)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        title={d.status === "normal" ? "禁用" : "启用"}
                        onClick={() => void toggleStatus(d)}
                      >
                        {d.status === "normal" ? <Ban className="h-3.5 w-3.5" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        title="重新签发密钥"
                        onClick={() => void rotate(d)}
                      >
                        <KeyRound className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 text-destructive"
                        title="删除"
                        onClick={() => setDeleting(d)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}

              {!loading && devices.length === 0 && !error && (
                <TableRow>
                  <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                    还没有设备，点击右上角「签发新设备 Token」创建第一个
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* ── 新建 / 密钥展示 ───────────────────────────────── */}
      <Dialog open={creating} onOpenChange={(v) => { setCreating(v); if (!v) setIssued(null); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{issued ? "请立即保存密钥" : "签发新设备 Token"}</DialogTitle>
            <DialogDescription>
              {issued ? "该密钥只显示这一次，关闭后无法再次查看" : "密钥只展示一次，请立即保存"}
            </DialogDescription>
          </DialogHeader>
          {issued ? (
            <div className="space-y-3">
              <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-4">
                <div className="flex items-center gap-2 text-sm font-medium text-emerald-600">
                  <KeyRound className="h-4 w-4" /> 签发成功
                </div>
                <code className="mt-2 block break-all rounded bg-secondary p-2.5 font-mono text-xs">{issued}</code>
              </div>
              <Button
                variant="outline"
                size="sm"
                className="w-full"
                onClick={async () => { await navigator.clipboard.writeText(issued); toast("已复制"); }}
              >
                <Copy className="h-3.5 w-3.5" /> 复制 Token
              </Button>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="space-y-1.5">
                <Label>设备名称</Label>
                <Input
                  placeholder="例如：客厅电视盒子"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label>每分钟限流 (RPM)</Label>
                <Select value={newRpm} onValueChange={setNewRpm}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {RPM_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}
          <DialogFooter>
            {issued ? (
              <Button onClick={() => setCreating(false)}>完成</Button>
            ) : (
              <>
                <Button variant="outline" onClick={() => setCreating(false)}>取消</Button>
                <Button onClick={() => void create()} disabled={busy}>
                  {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 签发
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── 编辑 ─────────────────────────────────────────── */}
      <Dialog open={!!editing} onOpenChange={(v) => !v && setEditing(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑设备</DialogTitle>
            <DialogDescription>{editing?.id}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>设备名称</Label>
              <Input value={editName} onChange={(e) => setEditName(e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>每分钟限流 (RPM)</Label>
              <Select value={editRpm} onValueChange={setEditRpm}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {RPM_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
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

      {/* ── 删除二次确认 ─────────────────────────────────── */}
      <Dialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-red-500" /> 确认删除设备？
            </DialogTitle>
            <DialogDescription>
              设备 <span className="font-medium text-foreground">{deleting?.name}</span>（{deleting?.id}）
              将被永久删除，其密钥立即失效。此操作不可撤销。
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
