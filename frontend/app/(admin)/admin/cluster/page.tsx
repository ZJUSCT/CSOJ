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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

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
                                <TableHead>Queue Mode</TableHead>
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
                                    <TableCell><Badge variant={c.queue_mode === 'kueue' ? 'default' : 'outline'}>{c.queue_mode}</Badge></TableCell>
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
            {createOpen && <ClusterFormDialog open={createOpen} onOpenChange={setCreateOpen} mode="create" onSuccess={() => { void mutate(); void globalMutate('/admin/clusters/status'); }} />}
            {editingCluster && <ClusterFormDialog open={!!editingCluster} onOpenChange={(v) => !v && setEditingCluster(null)} mode="edit" cluster={editingCluster} onSuccess={() => { void mutate(); void globalMutate('/admin/clusters/status'); }} />}
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
        queue_mode: cluster?.queue_mode || 'channel',
    });

    const handleSave = async () => {
        try {
            let response;
            if (mode === 'create') {
                response = await api.post('/admin/clusters', form);
            } else {
                response = await api.put(`/admin/clusters/${form.name}`, form);
            }
            const warnings: string[] = response.data.data?.warnings ?? [];
            toast({
                title: warnings.length > 0
                    ? `Cluster ${mode === 'create' ? 'created' : 'updated'} with warnings`
                    : `Cluster ${mode === 'create' ? 'created' : 'updated'}`,
                description: warnings.length > 0 ? warnings.join('; ') : undefined,
            });
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
                        <div>
                            <Label>Queue Mode</Label>
                            <Select value={form.queue_mode} onValueChange={v => setForm({ ...form, queue_mode: v })}>
                                <SelectTrigger><SelectValue /></SelectTrigger>
                                <SelectContent>
                                    <SelectItem value="channel">channel</SelectItem>
                                    <SelectItem value="kueue">kueue</SelectItem>
                                </SelectContent>
                            </Select>
                            {form.queue_mode === 'kueue' && (
                                <p className="text-xs text-muted-foreground mt-1">
                                    Requires Kueue operator. Jobs use queue name <code className="font-mono">{form.name || '<name>'}-queue</code>.
                                </p>
                            )}
                        </div>
                    </div>
                    <div>
                        <Label>Kubeconfig {mode === 'edit' ? '(leave empty to keep existing)' : '(full YAML text)'}</Label>
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
                            <Badge variant={cluster.QueueMode === 'kueue' ? 'default' : 'outline'}>
                                {cluster.QueueMode === 'kueue' ? 'Kueue' : 'Channel'}
                            </Badge>
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
            <ClusterRowsSection />
            <PoolStatusSection />
        </div>
    );
}

export default ClusterPage;
