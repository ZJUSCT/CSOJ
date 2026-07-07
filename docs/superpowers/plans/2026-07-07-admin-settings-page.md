# Admin Settings Page + Cluster Page Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new `/admin/settings` page (tabbed forms for DB-managed runtime settings) and rewrite the `/admin/cluster` page to use the new cluster-row + pool CRUD endpoints.

**Architecture:** New settings page with 4 tabs (General/Security/GitLab/CORS) calling `GET/PUT /admin/settings`. Rewritten cluster page with cluster-row table + pool status table calling the DB-managed cluster/pool endpoints. A new "Settings" entry in the admin sub-nav.

**Tech Stack:** Next.js 14 App Router (static export), React 18, TypeScript, pnpm, shadcn/ui, Tailwind, SWR, axios.

**Reference spec:** `docs/superpowers/specs/2026-07-07-admin-settings-page-design.md`

**Branch:** `merge-webui-admin`.

---

## File Structure

- **Modify** `frontend/lib/types.ts` — add `ClusterRow`, `ClusterNodePool`, `PoolState`, `ClusterStateSnapshot`; update `ClusterStatusResponse`; remove dead `ConfigNode`/`NodeState`/`NodeDetail`
- **Modify** `frontend/components/layout/admin-sub-nav.tsx` — add "Settings" entry
- **Create** `frontend/app/(main)/admin/settings/page.tsx` — new settings page
- **Rewrite** `frontend/app/(main)/admin/cluster/page.tsx` — cluster rows + pool status

### Out of scope
- i18n (English-hardcoded, matching existing admin pages)
- Editing boot facts (config.yaml) from the UI
- Changes to other admin pages

---

## Task 1: Update types + add Settings to sub-nav

**Files:**
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/components/layout/admin-sub-nav.tsx`

- [ ] **Step 1: Replace the cluster types in `frontend/lib/types.ts`**

Find the block starting at `export interface ConfigNode` (around line 186) through `export interface NodeDetail extends NodeState` (around line 213). Replace the entire block with:

```ts
export interface ClusterRow {
  name: string;
  kubeconfig: string;
  context: string;
  namespace: string;
  concurrency: number;
  heartbeat_ttl: number;
}

export interface ClusterNodePool {
  cluster_name: string;
  pool_name: string;
  node_selector: Record<string, string>;
  cpu: number;
  memory: number;
  is_paused: boolean;
}

export interface PoolState {
  Name: string;
  NodeSelector: Record<string, string>;
  CPU: number;
  Memory: number;
  IsPaused: boolean;
}

export interface ClusterStateSnapshot {
  Name: string;
  Namespace: string;
  Pools: Record<string, PoolState>;
  MPIEnabled: boolean;
  QueueLength: number;
  Concurrency: number;
}

export interface ClusterStatusResponse {
  resource_status: Record<string, ClusterStateSnapshot>;
  queue_lengths: Record<string, number>;
}
```

- [ ] **Step 2: Add "Settings" to `frontend/components/layout/admin-sub-nav.tsx`**

Add `Settings` to the lucide-react import (it already imports `Server`, `Users`, `FileCode`, `Trophy`, `BookCopy`, `Package`). Then append to the `routes` array after the "Problems" entry:

```ts
    { href: "/admin/settings", label: "Settings", icon: Settings },
```

- [ ] **Step 3: Verify the frontend builds**

Run: `cd frontend && pnpm build`
Expected: succeeds (the old cluster page references `NodeDetail` which was removed — it will fail. That's fixed in Task 3. If the build fails ONLY on the cluster page, that's expected. If it fails on other pages too, investigate.)

Actually — to keep the build green per-commit, the old cluster page will break because it imports `NodeDetail` and `ClusterStatusResponse` (which changed). So this task and Task 3 must be committed together. **Do NOT commit yet.** Proceed to Task 2 (settings page), then Task 3 (cluster rewrite), then commit everything together.

Actually, simplest: commit types + nav now (the cluster page will break, but the build of just the types/nav files is fine). Then do the settings page (new file, doesn't depend on cluster types). Then the cluster rewrite. Three commits, but the middle one (settings page) compiles because it only uses the new types.

Wait — `pnpm build` builds ALL pages. The broken cluster page blocks the whole build. So the types change + cluster rewrite must be in one commit. Do:

1. Make the types change + nav change (uncommitted).
2. Do Task 2 (settings page) — uncommitted.
3. Do Task 3 (cluster rewrite) — uncommitted.
4. Commit all three together: `feat(frontend): settings page + cluster rewrite + types`.

Or split into two commits: (a) types + cluster rewrite (build green), (b) settings page + nav (build green). Do (a) first.

**Revised plan:**
- Task 1: types + cluster page rewrite (one commit, build green).
- Task 2: settings page + sub-nav (one commit, build green).

- [ ] **Step 3 (revised): Skip commit here — proceed to Task 2, then Task 3, then commit together.**

---

## Task 2: Rewrite the cluster page

**Files:**
- Rewrite: `frontend/app/(main)/admin/cluster/page.tsx`

- [ ] **Step 1: Replace the entire cluster page**

Replace `frontend/app/(main)/admin/cluster/page.tsx` with:

```tsx
"use client";

import { useState } from 'react';
import useSWR from 'swr';
import api from '@/lib/api';
import { ClusterRow, ClusterNodePool, ClusterStatusResponse } from '@/lib/types';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import { Badge } from '@/components/ui/badge';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogFooter } from '@/components/ui/dialog';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '@/components/ui/alert-dialog';
import { useToast } from '@/hooks/use-toast';
import { useSWRConfig } from 'swr';
import { Server, PlusCircle, RefreshCw, Trash2, Edit, Pause, Play, Settings2 } from 'lucide-react';
import withAdmin from '@/components/layout/with-admin';
import { AdminSubNav } from '@/components/layout/admin-sub-nav';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

function ClusterRowsSection() {
    const { data: clusters, isLoading, mutate } = useSWR<ClusterRow[]>('/admin/clusters', fetcher);
    const { toast } = useToast();
    const { mutate: globalMutate } = useSWRConfig();
    const [editingCluster, setEditingCluster] = useState<ClusterRow | null>(null);
    const [createOpen, setCreateOpen] = useState(false);

    const handleReload = async () => {
        try {
            const res = await api.post('/admin/clusters/reload');
            const warnings = res.data.data?.warnings;
            if (warnings && warnings.length > 0) {
                toast({ title: 'Reload completed with warnings', description: warnings.join('; ') });
            } else {
                toast({ title: 'Clusters reloaded successfully' });
            }
            mutate();
            globalMutate('/admin/clusters/status');
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Reload failed', description: err.response?.data?.message });
        }
    };

    if (isLoading) return <Skeleton className="h-48 w-full" />;

    return (
        <Card>
            <CardHeader>
                <div className="flex items-center justify-between">
                    <div>
                        <CardTitle className="flex items-center gap-2"><Server /> K8s Clusters</CardTitle>
                        <CardDescription>Manage cluster connections (kubeconfig stored in DB)</CardDescription>
                    </div>
                    <div className="flex gap-2">
                        <Button variant="outline" size="sm" onClick={handleReload}><RefreshCw /> Reload</Button>
                        <Button size="sm" onClick={() => setCreateOpen(true)}><PlusCircle /> Add Cluster</Button>
                    </div>
                </div>
            </CardHeader>
            <CardContent>
                {(!clusters || clusters.length === 0) ? (
                    <p className="text-muted-foreground text-sm">No clusters configured. Add one to enable judging.</p>
                ) : (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead>Name</TableHead>
                                <TableHead>Namespace</TableHead>
                                <TableHead>Concurrency</TableHead>
                                <TableHead>Heartbeat TTL (s)</TableHead>
                                <TableHead className="text-right">Actions</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {clusters.map(c => (
                                <TableRow key={c.name}>
                                    <TableCell className="font-medium">{c.name}</TableCell>
                                    <TableCell>{c.namespace}</TableCell>
                                    <TableCell>{c.concurrency}</TableCell>
                                    <TableCell>{c.heartbeat_ttl}</TableCell>
                                    <TableCell className="text-right space-x-2">
                                        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => setEditingCluster(c)}>
                                            <Edit className="h-4 w-4" />
                                        </Button>
                                        <AlertDialog>
                                            <AlertDialogTrigger asChild>
                                                <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive">
                                                    <Trash2 className="h-4 w-4" />
                                                </Button>
                                            </AlertDialogTrigger>
                                            <AlertDialogContent>
                                                <AlertDialogHeader>
                                                    <AlertDialogTitle>Delete cluster "{c.name}"?</AlertDialogTitle>
                                                    <AlertDialogDescription>This will remove the cluster and its pool configurations. Judging will stop for this cluster.</AlertDialogDescription>
                                                </AlertDialogHeader>
                                                <AlertDialogFooter>
                                                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                                                    <AlertDialogAction className="bg-destructive" onClick={async () => {
                                                        try {
                                                            await api.delete(`/admin/clusters/${c.name}`);
                                                            toast({ title: 'Cluster deleted' });
                                                            mutate();
                                                        } catch (err: any) {
                                                            toast({ variant: 'destructive', title: 'Delete failed', description: err.response?.data?.message });
                                                        }
                                                    }}>Delete</AlertDialogAction>
                                                </AlertDialogFooter>
                                            </AlertDialogContent>
                                        </AlertDialog>
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                )}
            </CardContent>
            {createOpen && <ClusterFormDialog open={createOpen} onOpenChange={setCreateOpen} mode="create" onSuccess={() => mutate()} />}
            {editingCluster && <ClusterFormDialog open={!!editingCluster} onOpenChange={(v) => !v && setEditingCluster(null)} mode="edit" cluster={editingCluster} onSuccess={() => mutate()} />}
        </Card>
    );
}

function ClusterFormDialog({ open, onOpenChange, mode, cluster, onSuccess }: {
    open: boolean;
    onOpenChange: (v: boolean) => void;
    mode: 'create' | 'edit';
    cluster?: ClusterRow;
    onSuccess: () => void;
}) {
    const { toast } = useToast();
    const [form, setForm] = useState({
        name: cluster?.name || '',
        kubeconfig: cluster?.kubeconfig || '',
        context: cluster?.context || '',
        namespace: cluster?.namespace || 'csoj-judger',
        concurrency: cluster?.concurrency || 4,
        heartbeat_ttl: cluster?.heartbeat_ttl || 30,
    });

    const handleSave = async () => {
        try {
            if (mode === 'create') {
                await api.post('/admin/clusters', form);
            } else {
                await api.put(`/admin/clusters/${form.name}`, form);
            }
            toast({ title: `Cluster ${mode === 'create' ? 'created' : 'updated'}` });
            onOpenChange(false);
            onSuccess();
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Failed', description: err.response?.data?.message });
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
                <DialogHeader>
                    <DialogTitle>{mode === 'create' ? 'Add Cluster' : 'Edit Cluster'}</DialogTitle>
                </DialogHeader>
                <div className="space-y-4">
                    <div className="grid grid-cols-2 gap-4">
                        <div>
                            <Label>Name</Label>
                            <Input value={form.name} disabled={mode === 'edit'} onChange={e => setForm({ ...form, name: e.target.value })} placeholder="my-cluster" />
                        </div>
                        <div>
                            <Label>Context (optional)</Label>
                            <Input value={form.context} onChange={e => setForm({ ...form, context: e.target.value })} placeholder="(default context)" />
                        </div>
                        <div>
                            <Label>Namespace</Label>
                            <Input value={form.namespace} onChange={e => setForm({ ...form, namespace: e.target.value })} />
                        </div>
                        <div>
                            <Label>Concurrency</Label>
                            <Input type="number" value={form.concurrency} onChange={e => setForm({ ...form, concurrency: parseInt(e.target.value) || 1 })} />
                        </div>
                        <div>
                            <Label>Heartbeat TTL (seconds)</Label>
                            <Input type="number" value={form.heartbeat_ttl} onChange={e => setForm({ ...form, heartbeat_ttl: parseInt(e.target.value) || 30 })} />
                        </div>
                    </div>
                    <div>
                        <Label>Kubeconfig (full YAML text)</Label>
                        <Textarea
                            className="font-mono text-xs min-h-[200px]"
                            value={form.kubeconfig}
                            onChange={e => setForm({ ...form, kubeconfig: e.target.value })}
                            placeholder="apiVersion: v1&#10;clusters:&#10;- cluster:&#10;    certificate-authority-data: ...&#10;    server: https://..."
                        />
                    </div>
                </div>
                <DialogFooter>
                    <Button onClick={handleSave}>Save</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function PoolStatusSection() {
    const { data, isLoading } = useSWR<ClusterStatusResponse>('/admin/clusters/status', fetcher, { refreshInterval: 3000 });

    if (isLoading) return <Skeleton className="h-48 w-full" />;
    if (!data || Object.keys(data.resource_status).length === 0) {
        return <Card><CardContent className="py-8 text-center text-muted-foreground">No cluster status available. Add a cluster and reload.</CardContent></Card>;
    }

    return (
        <div className="space-y-6">
            {Object.entries(data.resource_status).map(([clusterName, cluster]) => (
                <Card key={clusterName}>
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2">
                            {clusterName.toUpperCase()} Cluster
                            {cluster.MPIEnabled && <Badge variant="secondary">MPI</Badge>}
                        </CardTitle>
                        <CardDescription>
                            Namespace: {cluster.Namespace} | Queue: {cluster.QueueLength} | Concurrency: {cluster.Concurrency}
                        </CardDescription>
                    </CardHeader>
                    <CardContent>
                        <PoolTable clusterName={clusterName} pools={cluster.Pools} />
                    </CardContent>
                </Card>
            ))}
        </div>
    );
}

function PoolTable({ clusterName, pools }: { clusterName: string; pools: Record<string, any> }) {
    const { toast } = useToast();
    const { mutate: globalMutate } = useSWRConfig();
    const [editingPool, setEditingPool] = useState<string | null>(null);

    const handleTogglePause = async (poolName: string, currentPaused: boolean) => {
        try {
            await api.put(`/admin/clusters/${clusterName}/pools/${poolName}`, { is_paused: !currentPaused });
            toast({ title: `Pool ${!currentPaused ? 'paused' : 'resumed'}` });
            globalMutate('/admin/clusters/status');
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Action failed', description: err.response?.data?.message });
        }
    };

    const handleDelete = async (poolName: string) => {
        try {
            await api.delete(`/admin/clusters/${clusterName}/pools/${poolName}`);
            toast({ title: 'Pool deleted' });
            globalMutate('/admin/clusters/status');
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Delete failed', description: err.response?.data?.message });
        }
    };

    return (
        <Table>
            <TableHeader>
                <TableRow>
                    <TableHead>Pool</TableHead>
                    <TableHead>CPU</TableHead>
                    <TableHead>Memory (MB)</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                </TableRow>
            </TableHeader>
            <TableBody>
                {Object.entries(pools).map(([poolName, pool]: [string, any]) => (
                    <TableRow key={poolName}>
                        <TableCell className="font-medium">{poolName}</TableCell>
                        <TableCell>{pool.CPU}</TableCell>
                        <TableCell>{pool.Memory}</TableCell>
                        <TableCell>
                            {pool.IsPaused ? <Badge variant="destructive">Paused</Badge> : <Badge variant="default">Active</Badge>}
                        </TableCell>
                        <TableCell className="text-right space-x-1">
                            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => handleTogglePause(poolName, pool.IsPaused)}>
                                {pool.IsPaused ? <Play className="h-4 w-4" /> : <Pause className="h-4 w-4" />}
                            </Button>
                            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => setEditingPool(poolName)}>
                                <Edit className="h-4 w-4" />
                            </Button>
                            <AlertDialog>
                                <AlertDialogTrigger asChild>
                                    <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive">
                                        <Trash2 className="h-4 w-4" />
                                    </Button>
                                </AlertDialogTrigger>
                                <AlertDialogContent>
                                    <AlertDialogHeader>
                                        <AlertDialogTitle>Delete pool "{poolName}"?</AlertDialogTitle>
                                        <AlertDialogDescription>This removes the pool configuration from cluster "{clusterName}".</AlertDialogDescription>
                                    </AlertDialogHeader>
                                    <AlertDialogFooter>
                                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                                        <AlertDialogAction className="bg-destructive" onClick={() => handleDelete(poolName)}>Delete</AlertDialogAction>
                                    </AlertDialogFooter>
                                </AlertDialogContent>
                            </AlertDialog>
                        </TableCell>
                    </TableRow>
                ))}
            </TableBody>
        </Table>
    );
}

function ClusterPage() {
    return (
        <div className="space-y-6">
            <AdminSubNav />
            <h1 className="text-3xl font-bold">Cluster Management</h1>
            <ClusterRowsSection />
            <PoolStatusSection />
        </div>
    );
}

export default withAdmin(ClusterPage);
```

- [ ] **Step 2: Verify the frontend builds**

Run: `cd frontend && pnpm build`
Expected: succeeds. The cluster page now uses the new types and endpoints. (The settings page doesn't exist yet — that's Task 3.)

- [ ] **Step 3: Commit**

```bash
cd ..
git add frontend/lib/types.ts "frontend/app/(main)/admin/cluster/page.tsx"
git commit -m "feat(frontend): rewrite cluster page for DB-managed clusters + pools"
```

---

## Task 3: New settings page + sub-nav entry

**Files:**
- Create: `frontend/app/(main)/admin/settings/page.tsx`
- Modify: `frontend/components/layout/admin-sub-nav.tsx`

- [ ] **Step 1: Create the settings page**

Create `frontend/app/(main)/admin/settings/page.tsx`:

```tsx
"use client";

import { useState, useEffect } from 'react';
import useSWR from 'swr';
import api from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Checkbox } from '@/components/ui/checkbox';
import { useToast } from '@/hooks/use-toast';
import withAdmin from '@/components/layout/with-admin';
import { AdminSubNav } from '@/components/layout/admin-sub-nav';
import { Save, RotateCcw } from 'lucide-react';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

interface SettingsResponse {
    settings: Record<string, string>;
    boot: {
        listen: string;
        storage: { Database: string; SubmissionContent: string; SubmissionLog: string; UserAvatar: string };
        jwt_secret_present: boolean;
    };
}

function SettingsPage() {
    const { data, isLoading, mutate } = useSWR<SettingsResponse>('/admin/settings', fetcher);

    if (isLoading || !data) return (
        <div className="space-y-6">
            <AdminSubNav />
            <Skeleton className="h-96 w-full" />
        </div>
    );

    return (
        <div className="space-y-6">
            <AdminSubNav />
            <h1 className="text-3xl font-bold">System Settings</h1>

            {/* Boot Facts (read-only) */}
            <Card>
                <CardHeader>
                    <CardTitle>Boot Configuration (config.yaml — restart required to change)</CardTitle>
                </CardHeader>
                <CardContent className="grid grid-cols-2 gap-4 text-sm">
                    <div><span className="text-muted-foreground">Listen:</span> {data.boot.listen}</div>
                    <div><span className="text-muted-foreground">Database:</span> {data.boot.storage.Database}</div>
                    <div><span className="text-muted-foreground">Submission Content:</span> {data.boot.storage.SubmissionContent}</div>
                    <div><span className="text-muted-foreground">Submission Log:</span> {data.boot.storage.SubmissionLog}</div>
                    <div><span className="text-muted-foreground">User Avatar:</span> {data.boot.storage.UserAvatar}</div>
                    <div><span className="text-muted-foreground">JWT Secret:</span> <Badge variant={data.boot.jwt_secret_present ? "default" : "destructive"}>{data.boot.jwt_secret_present ? "Set" : "MISSING"}</Badge></div>
                </CardContent>
            </Card>

            <Tabs defaultValue="general">
                <TabsList className="grid w-full grid-cols-4">
                    <TabsTrigger value="general">General</TabsTrigger>
                    <TabsTrigger value="security">Security</TabsTrigger>
                    <TabsTrigger value="gitlab">GitLab OIDC</TabsTrigger>
                    <TabsTrigger value="cors">CORS</TabsTrigger>
                </TabsList>

                <TabsContent value="general">
                    <GeneralTab settings={data.settings} onSaved={() => mutate()} />
                </TabsContent>
                <TabsContent value="security">
                    <SecurityTab settings={data.settings} onSaved={() => mutate()} />
                </TabsContent>
                <TabsContent value="gitlab">
                    <GitLabTab settings={data.settings} onSaved={() => mutate()} />
                </TabsContent>
                <TabsContent value="cors">
                    <CORSTab settings={data.settings} onSaved={() => mutate()} />
                </TabsContent>
            </Tabs>
        </div>
    );
}

function parseSetting<T>(settings: Record<string, string>, key: string, fallback: T): T {
    const raw = settings[key];
    if (!raw) return fallback;
    try { return JSON.parse(raw) as T; } catch { return fallback; }
}

function GeneralTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const logger = parseSetting(settings, 'logger', { level: 'info', file: '' });
    const [level, setLevel] = useState(logger.level);
    const [file, setFile] = useState(logger.file);

    const handleSave = async () => {
        try {
            await api.put('/admin/settings/logger', { value: { level, file } });
            toast({ title: 'Logger updated', description: 'Restart required for the change to take effect.' });
            onSaved();
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Update failed', description: err.response?.data?.message });
        }
    };

    return (
        <Card>
            <CardHeader>
                <CardTitle>Logger <Badge variant="secondary">restart required</Badge></CardTitle>
                <CardDescription>Log level and output file path</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <Label>Log Level</Label>
                    <Select value={level} onValueChange={setLevel}>
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                            <SelectItem value="debug">debug</SelectItem>
                            <SelectItem value="info">info</SelectItem>
                            <SelectItem value="warn">warn</SelectItem>
                            <SelectItem value="error">error</SelectItem>
                        </SelectContent>
                    </Select>
                </div>
                <div>
                    <Label>Log File (optional, empty = stdout only)</Label>
                    <Input value={file} onChange={e => setFile(e.target.value)} placeholder="csoj.log" />
                </div>
                <Button onClick={handleSave}><Save className="h-4 w-4 mr-2" /> Save</Button>
            </CardContent>
        </Card>
    );
}

function SecurityTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const local = parseSetting(settings, 'auth.local', { enabled: true });
    const [localEnabled, setLocalEnabled] = useState(local.enabled);
    const expireHours = parseSetting(settings, 'auth.jwt.expire_hours', 72);
    const [expire, setExpire] = useState(expireHours);

    const handleSaveLocal = async () => {
        try {
            await api.put('/admin/settings/auth.local', { value: { enabled: localEnabled } });
            toast({ title: 'Local auth setting updated' });
            onSaved();
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Update failed', description: err.response?.data?.message });
        }
    };

    const handleSaveExpire = async () => {
        try {
            await api.put('/admin/settings/auth.jwt.expire_hours', { value: expire });
            toast({ title: 'JWT expiry updated' });
            onSaved();
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Update failed', description: err.response?.data?.message });
        }
    };

    return (
        <div className="space-y-4">
            <Card>
                <CardHeader>
                    <CardTitle>Local Authentication <Badge variant="default">live</Badge></CardTitle>
                    <CardDescription>Enable/disable username/password registration and login</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                    <div className="flex items-center space-x-2">
                        <Checkbox id="local-enabled" checked={localEnabled} onCheckedChange={(v) => setLocalEnabled(!!v)} />
                        <Label htmlFor="local-enabled">Enable local auth</Label>
                    </div>
                    <Button onClick={handleSaveLocal}><Save className="h-4 w-4 mr-2" /> Save</Button>
                </CardContent>
            </Card>
            <Card>
                <CardHeader>
                    <CardTitle>JWT Expiry <Badge variant="default">live</Badge></CardTitle>
                    <CardDescription>JWT token expiration in hours</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                    <div>
                        <Label>Expire Hours</Label>
                        <Input type="number" value={expire} onChange={e => setExpire(parseInt(e.target.value) || 72)} />
                    </div>
                    <Button onClick={handleSaveExpire}><Save className="h-4 w-4 mr-2" /> Save</Button>
                </CardContent>
            </Card>
        </div>
    );
}

function GitLabTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const gl = parseSetting(settings, 'auth.gitlab', { url: '', client_id: '', client_secret: '', redirect_uri: '', frontend_callback_url: '' });
    const [form, setForm] = useState(gl);

    const handleSave = async () => {
        try {
            await api.put('/admin/settings/auth.gitlab', { value: form });
            toast({ title: 'GitLab OIDC settings updated' });
            onSaved();
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Update failed', description: err.response?.data?.message });
        }
    };

    return (
        <Card>
            <CardHeader>
                <CardTitle>GitLab OIDC <Badge variant="default">live</Badge></CardTitle>
                <CardDescription>Configure GitLab OAuth2/OIDC authentication</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <Label>OIDC Provider URL</Label>
                    <Input value={form.url} onChange={e => setForm({ ...form, url: e.target.value })} placeholder="https://gitlab.com" />
                </div>
                <div>
                    <Label>Client ID</Label>
                    <Input value={form.client_id} onChange={e => setForm({ ...form, client_id: e.target.value })} />
                </div>
                <div>
                    <Label>Client Secret</Label>
                    <Input type="password" value={form.client_secret} onChange={e => setForm({ ...form, client_secret: e.target.value })} />
                </div>
                <div>
                    <Label>Redirect URI</Label>
                    <Input value={form.redirect_uri} onChange={e => setForm({ ...form, redirect_uri: e.target.value })} placeholder="https://oj.example.com/api/v1/auth/gitlab/callback" />
                </div>
                <div>
                    <Label>Frontend Callback URL</Label>
                    <Input value={form.frontend_callback_url} onChange={e => setForm({ ...form, frontend_callback_url: e.target.value })} placeholder="/callback" />
                </div>
                <Button onClick={handleSave}><Save className="h-4 w-4 mr-2" /> Save</Button>
            </CardContent>
        </Card>
    );
}

function CORSTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const cors = parseSetting(settings, 'cors', { allowed_origins: [] as string[] });
    const [origins, setOrigins] = useState(cors.allowed_origins.join('\n'));

    const handleSave = async () => {
        const list = origins.split('\n').map(s => s.trim()).filter(Boolean);
        try {
            await api.put('/admin/settings/cors', { value: { allowed_origins: list } });
            toast({ title: 'CORS settings updated' });
            onSaved();
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Update failed', description: err.response?.data?.message });
        }
    };

    return (
        <Card>
            <CardHeader>
                <CardTitle>CORS <Badge variant="default">live</Badge></CardTitle>
                <CardDescription>Allowed origins for cross-origin requests (one per line)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <Label>Allowed Origins</Label>
                    <Textarea
                        className="font-mono text-sm min-h-[100px]"
                        value={origins}
                        onChange={e => setOrigins(e.target.value)}
                        placeholder="https://oj.example.com&#10;https://admin.oj.example.com&#10;*"
                    />
                </div>
                <Button onClick={handleSave}><Save className="h-4 w-4 mr-2" /> Save</Button>
            </CardContent>
        </Card>
    );
}

export default withAdmin(SettingsPage);
```

- [ ] **Step 2: Add "Settings" to the admin sub-nav**

In `frontend/components/layout/admin-sub-nav.tsx`, add `Settings` to the lucide-react import. Then append to the `routes` array after the "Problems" entry:

```ts
    { href: "/admin/settings", label: "Settings", icon: Settings },
```

- [ ] **Step 3: Verify the frontend builds**

Run: `cd frontend && pnpm build`
Expected: succeeds. 18 static pages (the existing 17 + the new settings page). The `/admin/settings` route appears in the route table.

- [ ] **Step 4: Commit**

```bash
cd ..
git add "frontend/app/(main)/admin/settings/page.tsx" frontend/components/layout/admin-sub-nav.tsx
git commit -m "feat(frontend): admin settings page + sub-nav entry"
```

---

## Task 4: Build + verify

**Files:** (no changes — verification only)

- [ ] **Step 1: Build the frontend + backend**

Run: `cd frontend && pnpm build`
Expected: succeeds, 18 pages including `/admin/settings`.

Run: `cd .. && make build`
Expected: `CSOJ` binary produced.

- [ ] **Step 2: Manual smoke test**

Start the server with a shrunk config (listen + storage + jwt.secret only). Login as superadmin. Navigate to `/admin/settings` — verify the 4 tabs + boot-facts card. Change CORS, save, verify it takes effect. Disable local auth, verify `/auth/status` reflects it. Navigate to `/admin/cluster` — verify the cluster table (empty initially), the "Add Cluster" dialog with kubeconfig textarea, and the "Reload" button.

- [ ] **Step 3: No commit**

---

## Self-Review notes

- The `ClusterStatusResponse` type uses Go's exported field names (`Name`, `Namespace`, `Pools`, `MPIEnabled`, `QueueLength`, `Concurrency`, `IsPaused`, `CPU`, `Memory`, `NodeSelector`) because the scheduler's `ClusterStateSnapshot` struct uses capitalized Go field names that serialize directly to JSON. The frontend accesses these as `pool.CPU`, `pool.Memory`, `pool.IsPaused`, etc.
- The settings page's `SettingsResponse` interface uses `boot.storage` with Go-style field names (`Database`, `SubmissionContent`, etc.) because the backend's `config.Storage` struct has capitalized Go field names.
- The `parseSetting` helper safely handles missing/empty settings rows (returns a fallback), matching the backend's `SettingsStore.Get` which returns zero-value on miss.
- The cluster page's pool table uses `any` types for the pool entries (`Record<string, any>`) because the `PoolState` type has capitalized fields that TS infers from the Go struct. The page accesses `pool.CPU`, `pool.Memory`, `pool.IsPaused` directly. A typed version would use `Record<string, PoolState>` — but the `any` is simpler for a page that just reads a few fields.
