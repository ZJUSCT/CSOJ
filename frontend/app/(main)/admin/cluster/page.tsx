"use client";

import useSWR from 'swr';
import api from '@/lib/api';
import { ClusterStatusResponse, NodeDetail } from '@/lib/types';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { useToast } from '@/hooks/use-toast';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import { formatBytes } from '@/lib/utils';
import { Play, Pause, Server, Cpu, MemoryStick, Info, Layers } from 'lucide-react';
import withAdmin from "@/components/layout/with-admin";
import { AdminSubNav } from "@/components/layout/admin-sub-nav";

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

function NodeDetails({ clusterName, nodeName }: { clusterName: string, nodeName: string }) {
    const { data: details, isLoading } = useSWR<NodeDetail>(`/admin/clusters/${clusterName}/nodes/${nodeName}`, fetcher);

    if (isLoading || !details) return <Skeleton className="h-48 w-full" />;

    const totalCores = details.cpu;
    const usedCoresCount = details.used_cores.filter(Boolean).length;

    return (
        <div className="space-y-4">
            <h3 className="font-semibold">{details.name}</h3>
            <div className="grid grid-cols-2 gap-4 text-sm">
                <div><strong>Docker Host:</strong> <span className="font-mono">{details.docker.host}</span></div>
                <div><strong>Paused:</strong> {details.is_paused ? "Yes" : "No"}</div>
                <div><strong>Total Memory:</strong> {formatBytes(details.memory * 1024 * 1024)}</div>
                <div><strong>Used Memory:</strong> {formatBytes(details.used_memory * 1024 * 1024)}</div>
                <div><strong>Total Cores:</strong> {totalCores}</div>
                <div><strong>Used Cores:</strong> {usedCoresCount}</div>
            </div>
            <p className="text-sm font-semibold">Core Usage:</p>
            <div className="flex flex-wrap gap-2">
                {details.used_cores.map((isUsed, i) => (
                    <div key={i} title={`Core ${i}`} className={`h-6 w-6 rounded flex items-center justify-center text-xs ${isUsed ? 'bg-blue-500 text-white' : 'bg-muted'}`}>
                        {i}
                    </div>
                ))}
            </div>
        </div>
    );
}


function ClusterStatusPage() {
    const { data, error, isLoading, mutate } = useSWR<ClusterStatusResponse>('/admin/clusters/status', fetcher, { refreshInterval: 3000 });
    const { toast } = useToast();

    const handleNodeAction = async (clusterName: string, nodeName: string, action: 'pause' | 'resume') => {
        try {
            await api.post(`/admin/clusters/${clusterName}/nodes/${nodeName}/${action}`);
            toast({
                title: 'Success',
                description: `Node ${nodeName} has been ${action}d.`,
            });
            mutate();
        } catch (err: any) {
            toast({
                variant: 'destructive',
                title: 'Action Failed',
                description: err.response?.data?.message || `Could not ${action} the node.`,
            });
        }
    };

    if (isLoading) return <div className="space-y-4">
        <AdminSubNav />
        <Skeleton className="h-8 w-1/4" />
        <Skeleton className="h-64 w-full" />
        <Skeleton className="h-64 w-full" />
    </div>;
    if (error) return <div className="space-y-6"><AdminSubNav /><p>Failed to load cluster status.</p></div>;
    if (!data) return <div className="space-y-6"><AdminSubNav /><p>No cluster data available.</p></div>;

    return (
        <div className="space-y-6">
            <AdminSubNav />
            <h1 className="text-3xl font-bold">Cluster Status</h1>
            {Object.entries(data.resource_status).map(([clusterName, cluster]) => (
                <Card key={clusterName}>
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2">
                            <Layers />
                            {clusterName.toUpperCase()} Cluster
                        </CardTitle>
                        <CardDescription>
                            Queue Length: <span className="font-bold text-primary">{data.queue_lengths[clusterName] ?? 0}</span>
                        </CardDescription>
                    </CardHeader>
                    <CardContent>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>Node</TableHead>
                                    <TableHead>Status</TableHead>
                                    <TableHead>CPU Usage</TableHead>
                                    <TableHead>Memory Usage</TableHead>
                                    <TableHead>Actions</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {Object.values(cluster.nodes).map(node => {
                                    const usedCoresCount = node.used_cores?.filter(Boolean).length ?? '?';
                                    return (
                                        <TableRow key={node.name}>
                                            <TableCell className="font-medium flex items-center gap-2"><Server /> {node.name}</TableCell>
                                            <TableCell>{node.is_paused ? 'Paused' : 'Active'}</TableCell>
                                            <TableCell className="font-mono flex items-center gap-2"><Cpu /> {usedCoresCount} / {node.cpu}</TableCell>
                                            <TableCell>
                                                <div className="flex items-center gap-2">
                                                    <MemoryStick />
                                                    <span>{formatBytes(node.used_memory * 1024 * 1024)} / {formatBytes(node.memory * 1024 * 1024)}</span>
                                                </div>
                                            </TableCell>
                                            <TableCell className="space-x-2">
                                                <Dialog>
                                                    <DialogTrigger asChild>
                                                        <Button variant="outline" size="sm"><Info/> Details</Button>
                                                    </DialogTrigger>
                                                    <DialogContent>
                                                        <DialogHeader>
                                                            <DialogTitle>Node Details</DialogTitle>
                                                        </DialogHeader>
                                                        <NodeDetails clusterName={clusterName} nodeName={node.name} />
                                                    </DialogContent>
                                                </Dialog>

                                                {node.is_paused ? (
                                                    <Button size="sm" onClick={() => handleNodeAction(clusterName, node.name, 'resume')}>
                                                        <Play /> Resume
                                                    </Button>
                                                ) : (
                                                    <Button variant="destructive" size="sm" onClick={() => handleNodeAction(clusterName, node.name, 'pause')}>
                                                        <Pause /> Pause
                                                    </Button>
                                                )}
                                            </TableCell>
                                        </TableRow>
                                    );
                                })}
                            </TableBody>
                        </Table>
                    </CardContent>
                </Card>
            ))}
        </div>
    );
}

export default withAdmin(ClusterStatusPage);
