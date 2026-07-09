"use client"
import { useSearchParams } from 'next/navigation';
import useSWR, { useSWRConfig } from 'swr';
import api from '@/lib/api';
import { Problem, Submission, Contest } from '@/lib/types';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import Link from 'next/link';
import { Suspense } from 'react';
import SubmissionStatusBadge from '@/components/shared/submission-status-badge';
import { format } from 'date-fns';
import { Badge } from '@/components/ui/badge';
import { Bot, Calendar, Clock, Code2, Cpu, FolderSymlink, Hash, MemoryStick, Network, Server, Target, Trophy, UploadCloud, PlusCircle, Edit, Trash2, Star } from 'lucide-react';
import { formatBytes } from '@/lib/utils';
import React from 'react';
import { ProblemFormDialog, DeleteProblemButton } from '@/components/admin/problem-actions';
import { Button } from '@/components/ui/button';
import { useRouter } from 'next/navigation';
import { AssetManager } from '@/components/admin/asset-manager';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

function ProblemList() {
    const { data: problems, isLoading, mutate } = useSWR<Record<string, Problem>>('/admin/problems', fetcher);
    const { data: contests, isLoading: contestsLoading } = useSWR<Record<string, Contest>>('/admin/contests', fetcher);
    const { mutate: contestsMutate } = useSWRConfig();

    const onSuccess = () => {
        mutate();
        contestsMutate('/admin/contests');
    };

    if (isLoading || contestsLoading) return <Skeleton className="h-64 w-full" />;

    const problemList = problems ? Object.values(problems) : [];

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">{problemList.length} problem(s)</p>
                <ProblemFormDialog contests={Object.values(contests || {})} onSuccess={onSuccess} trigger={<Button><PlusCircle/> Create Problem</Button>}/>
            </div>
            {problemList.length === 0 && (
                <Card>
                    <CardContent className="py-8 text-center text-muted-foreground">
                        No problems yet. Click "Create Problem" to add one.
                    </CardContent>
                </Card>
            )}
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
                {problemList.map(p => {
                    const parentContest = Object.values(contests || {}).find(c => c.problem_ids.includes(p.id));
                    return (
                        <Card key={p.id} className="flex flex-col">
                            <CardHeader>
                                <div className="flex items-start justify-between">
                                    <div className="flex-1 min-w-0">
                                        <Link href={`/admin/problems?id=${p.id}`} className="hover:underline">
                                            <CardTitle className="text-base truncate">{p.name}</CardTitle>
                                        </Link>
                                    </div>
                                    <Badge variant="secondary" className="ml-2 shrink-0">{p.level || 'N/A'}</Badge>
                                </div>
                            </CardHeader>
                            <CardContent className="flex-1 space-y-3">
                                <div className="flex items-center gap-3 text-xs text-muted-foreground">
                                    {parentContest && (
                                        <Link href={`/admin/contests?id=${parentContest.id}`} className="hover:underline flex items-center gap-1">
                                            <Trophy className="h-3 w-3" /> {parentContest.name}
                                        </Link>
                                    )}
                                    <span className="flex items-center gap-1"><Server className="h-3 w-3" /> {p.cluster}</span>
                                </div>
                                <div className="flex items-center gap-3 text-xs text-muted-foreground">
                                    <span className="flex items-center gap-1"><Server className="h-3 w-3" /> {p.cluster}</span>
                                    {p.workflow && p.workflow.length > 0 && (
                                        <span className="flex items-center gap-1"><Code2 className="h-3 w-3" /> {p.workflow.length} step(s)</span>
                                    )}
                                    {p.max_submissions != null && p.max_submissions > 0 && (
                                        <span className="flex items-center gap-1"><Hash className="h-3 w-3" /> {p.max_submissions} max</span>
                                    )}
                                </div>
                                <div className="flex items-center gap-2">
                                    <Badge variant="outline">{p.score.mode}</Badge>
                                    {p.workflow && p.workflow.map((step, i) => (
                                        <Badge key={i} variant="outline" className="text-xs">{step.name}</Badge>
                                    ))}
                                </div>
                            </CardContent>
                            <div className="flex items-center gap-2 p-4 pt-0">
                                <ProblemFormDialog problem={p} contestId={parentContest?.id} contests={Object.values(contests || {})} onSuccess={onSuccess} trigger={<Button variant="outline" size="sm" className="flex-1"><Edit className="h-3 w-3 mr-1" /> Edit</Button>}/>
                                <DeleteProblemButton problem={p} onSuccess={onSuccess} trigger={<Button variant="outline" size="sm" className="text-destructive"><Trash2 className="h-3 w-3" /></Button>}/>
                            </div>
                        </Card>
                    );
                })}
            </div>
        </div>
    );
}

const InfoItem = ({ icon: Icon, label, value }: { icon: React.ElementType, label: string, value: React.ReactNode }) => (
    <div className="flex items-start">
        <Icon className="h-4 w-4 mr-3 mt-1 flex-shrink-0 text-muted-foreground" />
        <div className="flex-1">
            <p className="text-muted-foreground">{label}</p>
            <div className="font-medium">{value}</div>
        </div>
    </div>
);


function ProblemDetails({ problemId }: { problemId: string }) {
    const { mutate } = useSWRConfig();
    const router = useRouter();

    const { data: problem, isLoading: problemLoading, mutate: mutateProblem } = useSWR<Problem>(`/admin/problems/${problemId}`, fetcher);
    const { data: contests, isLoading: contestsLoading } = useSWR<Record<string, Contest>>('/admin/contests', fetcher);
    const { data: submissionsData, isLoading: submissionsLoading } = useSWR<{ items: Submission[] }>(`/admin/submissions?problem_id=${problemId}&limit=100`, fetcher);
    const submissions = submissionsData?.items;

    if (problemLoading || contestsLoading || !problem || !contests) return <Skeleton className="h-screen w-full" />;

    const parentContest = Object.values(contests).find(c => c.problem_ids.includes(problem.id));

    const onSuccess = () => {
        mutateProblem();
        mutate(`/admin/contests`);
        mutate(`/admin/problems`);
    }

     const onProblemDelete = () => {
        mutate(`/admin/contests`);
        mutate(`/admin/problems`);
        router.push('/admin/problems');
    }

    return (
        <div className="space-y-6">
             <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
                <div>
                    <h1 className="text-3xl font-bold">{problem.name}</h1>
                    <p className="text-muted-foreground">Part of contest: <Link href={`/admin/contests?id=${parentContest?.id}`} className="text-primary hover:underline">{parentContest?.name}</Link></p>
                </div>
                <div className="flex items-center gap-2">
                    <ProblemFormDialog problem={problem} contestId={parentContest?.id} contests={Object.values(contests)} onSuccess={onSuccess} trigger={<Button variant="outline"><Edit/> Edit Problem</Button>}/>
                    <DeleteProblemButton problem={problem} onSuccess={onProblemDelete} trigger={<Button variant="destructive"><Trash2/> Delete Problem</Button>}/>
                </div>
            </div>

            <Card>
                <CardHeader>
                    <CardTitle>Problem Configuration</CardTitle>
                </CardHeader>
                <CardContent>
                    <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-x-8 gap-y-4 text-sm">
                        <InfoItem icon={Star} label="Level" value={problem.level || 'Not Set'} />
                        <InfoItem icon={Calendar} label="Start Time" value={format(new Date(problem.starttime), "Pp")} />
                        <InfoItem icon={Clock} label="End Time" value={format(new Date(problem.endtime), "Pp")} />
                        <InfoItem icon={Server} label="Cluster" value={problem.cluster} />
                        <InfoItem icon={Hash} label="Max Submissions" value={(problem.max_submissions ?? 0) > 0 ? problem.max_submissions : "Unlimited"} />
                        <InfoItem icon={Target} label="Score Mode" value={<Badge variant="secondary">{problem.score.mode}</Badge>} />
                        <InfoItem icon={UploadCloud} label="Upload Limit" value={`${problem.upload.max_num} file(s), ${formatBytes(problem.upload.max_size * 1024 * 1024)} max`} />
                    </div>
                </CardContent>
            </Card>

            <AssetManager assetType="problem" assetId={problemId} />

            <Card>
                <CardHeader><CardTitle>Workflow Steps</CardTitle></CardHeader>
                <CardContent className="space-y-4">
                    {problem.workflow.map((step, index) => (
                        <Card key={index} className="bg-muted/50">
                            <CardHeader>
                                <CardTitle className="text-lg flex flex-wrap items-center justify-between gap-2">
                                    <span>Step {index + 1}: {step.name}</span>
                                    <div className="flex items-center gap-2">
                                        {step.show && <Badge variant="outline">Logs Visible</Badge>}
                                        {step.root && <Badge variant="destructive">Run as Root</Badge>}
                                        {step.network && <Badge variant="default"><Network className="mr-1 h-3 w-3" /> Network</Badge>}
                                    </div>
                                </CardTitle>
                            </CardHeader>
                            <CardContent className="space-y-4">
                                <div>
                                    <h4 className="font-semibold flex items-center gap-2 mb-2"><Code2 /> Commands</h4>
                                    <div className="bg-background p-3 rounded-md font-mono text-xs space-y-1 overflow-x-auto">
                                        {step.steps.map((cmd, cmdIndex) => (
                                            <p key={cmdIndex}><span className="text-muted-foreground">$ </span>{cmd.join(' ')}</p>
                                        ))}
                                    </div>
                                </div>
                                {step.mounts && step.mounts.length > 0 && (
                                    <div>
                                        <h4 className="font-semibold flex items-center gap-2 mb-2"><FolderSymlink /> Mounts</h4>
                                        <div className="overflow-x-auto">
                                            <Table>
                                                <TableHeader><TableRow><TableHead>Source</TableHead><TableHead>Target</TableHead><TableHead>Type</TableHead><TableHead>Read Only</TableHead></TableRow></TableHeader>
                                                <TableBody>
                                                    {step.mounts.map((mount, mountIndex) => (
                                                        <TableRow key={mountIndex}>
                                                            <TableCell className="font-mono">{mount.source}</TableCell>
                                                            <TableCell className="font-mono">{mount.target}</TableCell>
                                                            <TableCell>{mount.type || 'bind'}</TableCell>
                                                            <TableCell>{mount.readonly === false ? 'No' : 'Yes'}</TableCell>
                                                        </TableRow>
                                                    ))}
                                                </TableBody>
                                            </Table>
                                        </div>
                                    </div>
                                )}
                            </CardContent>
                        </Card>
                    ))}
                </CardContent>
            </Card>

            <Card>
                <CardHeader><CardTitle>Recent Submissions</CardTitle></CardHeader>
                <CardContent>
                    {submissionsLoading ? <Skeleton className="h-48 w-full" /> : (
                        <Table>
                            <TableHeader><TableRow><TableHead>ID</TableHead><TableHead>User</TableHead><TableHead>Status</TableHead><TableHead>Score</TableHead><TableHead>Date</TableHead></TableRow></TableHeader>
                            <TableBody>
                                {submissions?.map(s => (
                                    <TableRow key={s.id}>
                                        <TableCell><Link href={`/admin/submissions?id=${s.id}`} className="font-mono text-primary hover:underline">{s.id.substring(0, 8)}...</Link></TableCell>
                                        <TableCell><Link href={`/admin/users?id=${s.user.id}`} className="hover:underline">{s.user.nickname}</Link></TableCell>
                                        <TableCell><SubmissionStatusBadge status={s.status} /></TableCell>
                                        <TableCell>{s.score}</TableCell>
                                        <TableCell>{format(new Date(s.CreatedAt), "Pp")}</TableCell>
                                    </TableRow>
                                ))}
                            </TableBody>
                        </Table>
                    )}
                </CardContent>
            </Card>
        </div>
    );
}

function ProblemsPageContent() {
    const searchParams = useSearchParams();
    const problemId = searchParams.get('id');

    return (
        <div className="space-y-6">
            {problemId ? <ProblemDetails problemId={problemId} /> : <ProblemList />}
        </div>
    );
}

function ProblemsPage() {
    return (
        <Suspense fallback={<Skeleton className="h-screen w-full" />}>
            <ProblemsPageContent />
        </Suspense>
    );
}

export default ProblemsPage;
