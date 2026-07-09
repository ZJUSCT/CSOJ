"use client";

import { useState } from 'react';
import useSWR, { useSWRConfig } from 'swr';
import { Contest, ContestRegistration } from '@/lib/types';
import api from '@/lib/api';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';
import { ClipboardCheck, Check, X } from 'lucide-react';
import { getTagColorClasses, cn } from '@/lib/utils';
import { format } from 'date-fns';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

type StatusFilter = 'all' | 'pending' | 'approved' | 'rejected';

function StatusBadge({ status }: { status: string }) {
    if (status === 'approved') return <Badge variant="default" className="bg-green-600 hover:bg-green-600">Approved</Badge>;
    if (status === 'pending') return <Badge variant="outline" className="border-yellow-500 text-yellow-600">Pending</Badge>;
    if (status === 'rejected') return <Badge variant="destructive">Rejected</Badge>;
    return <Badge variant="secondary">{status}</Badge>;
}

function RegistrationsTable({ contestId, statusFilter }: { contestId: string, statusFilter: StatusFilter }) {
    const { toast } = useToast();
    const { mutate: globalMutate } = useSWRConfig();
    const query = statusFilter === 'all' ? '' : `?status=${statusFilter}`;
    const { data: registrations, isLoading, mutate } = useSWR<ContestRegistration[]>(
        `/admin/contests/${contestId}/registrations${query}`,
        fetcher
    );
    const [actingId, setActingId] = useState<string | null>(null);

    const handleReview = async (reg: ContestRegistration, action: 'approve' | 'reject') => {
        setActingId(reg.id);
        try {
            await api.patch(`/admin/registrations/${reg.id}`, { action });
            toast({
                title: action === 'approve' ? 'Registration Approved' : 'Registration Rejected',
                description: `${reg.user.nickname} has been ${action === 'approve' ? 'approved' : 'rejected'}.`
            });
            mutate();
            globalMutate(`/admin/contests/${contestId}/registrations`);
            globalMutate(`/admin/contests/${contestId}/registrations?status=pending`);
            globalMutate(`/admin/contests/${contestId}/registrations?status=approved`);
            globalMutate(`/admin/contests/${contestId}/registrations?status=rejected`);
        } catch (err: any) {
            toast({
                variant: 'destructive',
                title: 'Action Failed',
                description: err.response?.data?.message || 'An unexpected error occurred.'
            });
        } finally {
            setActingId(null);
        }
    };

    if (isLoading) return <Skeleton className="h-48 w-full" />;

    const regs = registrations || [];

    if (regs.length === 0) {
        return (
            <Card>
                <CardContent className="py-8 text-center text-muted-foreground">
                    No registrations found for the selected filter.
                </CardContent>
            </Card>
        );
    }

    return (
        <Card>
            <CardHeader>
                <CardTitle className="flex items-center gap-2"><ClipboardCheck /> Registrations</CardTitle>
                    Review and approve or reject user registrations. Showing {regs.length} {statusFilter !== 'all' ? statusFilter : ''} registration(s).
            </CardHeader>
            <CardContent>
                <Table>
                    <TableHeader>
                        <TableRow>
                            <TableHead>User</TableHead>
                            <TableHead>Tags</TableHead>
                            <TableHead>Status</TableHead>
                            <TableHead>Registered At</TableHead>
                            <TableHead className="text-right">Actions</TableHead>
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {regs.map((reg) => (
                            <TableRow key={reg.id}>
                                <TableCell>
                                    <div className="flex flex-col">
                                        <span className="font-medium">{reg.user.nickname}</span>
                                        <span className="text-xs text-muted-foreground">@{reg.user.username}</span>
                                    </div>
                                </TableCell>
                                <TableCell>
                                    <div className="flex flex-wrap gap-1">
                                        {reg.user.tags ? (
                                            reg.user.tags.split(',').map(tag => {
                                                const trimmed = tag.trim();
                                                if (!trimmed) return null;
                                                return (
                                                    <Badge
                                                        key={trimmed}
                                                        variant="flat"
                                                        className={cn("text-xs border-transparent", getTagColorClasses(trimmed))}
                                                    >
                                                        {trimmed}
                                                    </Badge>
                                                );
                                            })
                                        ) : (
                                            <span className="text-xs text-muted-foreground">—</span>
                                        )}
                                    </div>
                                </TableCell>
                                <TableCell><StatusBadge status={reg.status} /></TableCell>
                                <TableCell className="text-sm text-muted-foreground">
                                    {format(new Date(reg.created_at), 'MMM d, yyyy HH:mm')}
                                </TableCell>
                                <TableCell className="text-right space-x-2">
                                    {reg.status === 'pending' ? (
                                        <>
                                            <Button
                                                size="sm"
                                                variant="default"
                                                className="bg-green-600 hover:bg-green-700"
                                                disabled={actingId === reg.id}
                                                onClick={() => handleReview(reg, 'approve')}
                                            >
                                                <Check className="mr-1 h-3 w-3" /> Approve
                                            </Button>
                                            <Button
                                                size="sm"
                                                variant="destructive"
                                                disabled={actingId === reg.id}
                                                onClick={() => handleReview(reg, 'reject')}
                                            >
                                                <X className="mr-1 h-3 w-3" /> Reject
                                            </Button>
                                        </>
                                    ) : (
                                        <span className="text-xs text-muted-foreground">No actions</span>
                                    )}
                                </TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
            </CardContent>
        </Card>
    );
}

function RegistrationsPageContent() {
    const { data: contests, isLoading: contestsLoading } = useSWR<Record<string, Contest>>('/admin/contests', fetcher);
    const [selectedContestId, setSelectedContestId] = useState<string>('');
    const [statusFilter, setStatusFilter] = useState<StatusFilter>('pending');

    // Auto-select the first contest once the list loads.
    const contestList = contests ? Object.values(contests) : [];
    const effectiveContestId = selectedContestId || (contestList.length > 0 ? contestList[0].id : '');

    return (
        <div className="space-y-6">
            <div className="flex items-center justify-between">
</div>

            <Card>
                <CardHeader>
                    <CardTitle>Filters</CardTitle>
                </CardHeader>
                <CardContent className="grid gap-4 md:grid-cols-2">
                    <div className="space-y-2">
                        <label className="text-sm font-medium">Contest</label>
                        <Select
                            value={effectiveContestId}
                            onValueChange={setSelectedContestId}
                            disabled={contestsLoading}
                        >
                            <SelectTrigger>
                                <SelectValue placeholder={contestsLoading ? 'Loading contests…' : 'Select a contest'} />
                            </SelectTrigger>
                            <SelectContent>
                                {contestList.map(c => (
                                    <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="space-y-2">
                        <label className="text-sm font-medium">Status</label>
                        <Select
                            value={statusFilter}
                            onValueChange={(v) => setStatusFilter(v as StatusFilter)}
                        >
                            <SelectTrigger>
                                <SelectValue placeholder="Filter by status" />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="all">All</SelectItem>
                                <SelectItem value="pending">Pending</SelectItem>
                                <SelectItem value="approved">Approved</SelectItem>
                                <SelectItem value="rejected">Rejected</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
                </CardContent>
            </Card>

            {effectiveContestId ? (
                <RegistrationsTable contestId={effectiveContestId} statusFilter={statusFilter} />
            ) : (
                <Card>
                    <CardContent className="py-8 text-center text-muted-foreground">
                        {contestsLoading ? 'Loading contests…' : 'No contests available. Create a contest first.'}
                    </CardContent>
                </Card>
            )}
        </div>
    );
}

function RegistrationsPage() {
    return <RegistrationsPageContent />;
}

export default RegistrationsPage;
