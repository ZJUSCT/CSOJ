"use client";
import useSWR from 'swr';
import api from '@/lib/api';
import { Problem, Submission } from '@/lib/types';
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import Link from 'next/link';
import SubmissionStatusBadge from '@/components/shared/submission-status-badge';
import { format, formatDistanceToNow } from 'date-fns';
import { zhCN, enUS, Locale } from "date-fns/locale";
import { useLocale } from "next-intl";
import { useSearchParams } from 'next/navigation';
import { Suspense } from 'react';
import { SubmissionLogViewer } from '@/components/submissions/submission-log-viewer';
import { Clock, Code, Hash, Layers, Loader2, Server, Tag, User, XCircle, Bookmark, Rocket } from 'lucide-react';
import { Progress } from "@/components/ui/progress";
import { Button } from '@/components/ui/button';
import { useToast } from '@/hooks/use-toast';
import { Separator } from '@/components/ui/separator';
import { useTranslations } from 'next-intl';
import { cn } from '@/lib/utils';
import { CopyButton } from "@/components/ui/shadcn-io/copy-button";
import { useRouter } from "next/navigation";
import { getScoreColor } from '@/lib/utils';
import MarkdownViewer from '@/components/shared/markdown-viewer';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

function MySubmissionsList() {
  const t = useTranslations('submissions');
  const router = useRouter();

  const { data: submissions, error, isLoading } = useSWR<Submission[]>('/submissions', fetcher, {
    refreshInterval: 5000,
  });

  if (isLoading)
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('list.title')}</CardTitle>
          <CardDescription>{t('list.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {[...Array(5)].map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        </CardContent>
      </Card>
    );

  if (error) return <div>{t('list.loadFail')}</div>;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-2xl font-bold">{t('list.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
                <TableHead>
                <div className="flex items-center gap-2">
                    <Hash className="h-4 w-4" />
                    {t('list.table.status')}
                </div>
                </TableHead>
                <TableHead>
                <div className="flex items-center gap-2">
                    <Tag className="h-4 w-4" />
                    {t('list.table.score')}
                </div>
                </TableHead>
                <TableHead>
                <div className="flex items-center gap-2">
                    <Tag className="h-4 w-4" />
                    {t('list.table.performance')}
                </div>
                </TableHead>
                <TableHead>
                <div className="flex items-center gap-2">
                    <Code className="h-4 w-4" />
                    {t('list.table.problemId')}
                </div>
                </TableHead>
                <TableHead>
                <div className="flex items-center gap-2">
                    <Server className="h-4 w-4" />
                    {t('list.table.node')}
                </div>
                </TableHead>
                <TableHead>
                <div className="flex items-center gap-2">
                    <Clock className="h-4 w-4" />
                    {t('list.table.submittedAt')}
                </div>
                </TableHead>
                <TableHead className="text-right">
                <div className="flex justify-end items-center gap-2">
                    <Bookmark className="h-4 w-4" />
                    {t('list.table.id')}
                </div>
                </TableHead>
            </TableRow>
            </TableHeader>
          <TableBody>
            {submissions && submissions.length > 0 ? (
              submissions.map((sub) => (
                <TableRow
                  key={sub.id}
                  className={cn(
                    "cursor-pointer transition-colors hover:bg-muted/50"
                  )}
                  onClick={() => router.push(`/submissions?id=${sub.id}`)}
                >
                  <TableCell>
                    <SubmissionStatusBadge status={sub.status} />
                  </TableCell>
                  <TableCell>
                    <span
                      className="font-bold font-mono"
                      style={{
                        color: sub.status === "Success" ? getScoreColor(sub.score ?? 0) : "",
                      }}
                    >
                      {sub.status === "Success" ? sub.score : "-"}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span
                      className="font-bold font-mono"
                    >
                      {sub.performance != 0 ? sub.performance.toFixed(2) : "-"}
                    </span>
                  </TableCell>
                  <TableCell>
                    {sub.problem_id}
                  </TableCell>
                  <TableCell>{sub.node}</TableCell>
                  <TableCell>
                    {format(new Date(sub.CreatedAt), "MM/dd HH:mm:ss")}
                  </TableCell>
                  <TableCell className="text-right font-mono text-sm text-muted-foreground">
                    <div className="flex items-center justify-end space-x-2">
                      <span className="mx-2">{sub.id.substring(0, 8)}</span>
                      <div
                        onClick={(e) => {
                          e.stopPropagation();
                        }}
                      >
                        <CopyButton content={sub.id} size="sm" />
                      </div>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={6} className="text-center">
                  {t('list.none')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function QueuePosition({ submissionId, cluster }: { submissionId: string, cluster: string }) {
    const t = useTranslations('submissions');
    const { data } = useSWR<{ position: number }>(`/submissions/${submissionId}/queue_position`, fetcher, { refreshInterval: 3000 });

    if (data === undefined) return null;

    return (
        <div className="flex items-center justify-between text-sm text-blue-500">
            <span className="text-muted-foreground flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />{t('details.queue.position')}</span>
            <span>{t('details.queue.info', { position: data.position + 1, cluster })}</span>
        </div>
    );
}


// Component for submission details
function SubmissionDetails({ submissionId }: { submissionId: string }) {
    const locale = useLocale();
    const locales: Record<string, Locale> = {
        zh: zhCN,
        en: enUS,
    };
    const t = useTranslations('submissions');
    const { toast } = useToast();
    const { data: submission, error: submissionError, isLoading: isSubmissionLoading, mutate } = useSWR<Submission>(`/submissions/${submissionId}`, fetcher, {
        refreshInterval: (data) => (data?.status === 'Queued' || data?.status === 'Running' ? 2000 : 0),
    });
    
    // Fetch problem data, but do not throw an error if it fails.
    // The `problem` variable will be `undefined` on 404, which is what we want.
    const { data: problem } = useSWR<Problem>(submission ? `/problems/${submission.problem_id}` : null, fetcher);

    if (isSubmissionLoading) return <SubmissionDetailsSkeleton />;
    // Only show error if the submission itself fails to load
    if (submissionError) return <div>{t('details.loadFail')}</div>;
    if (!submission) return <div>{t('details.notFound')}</div>;

    const markdownInfo = submission.info?.markdown;
    const remainingInfo = { ...submission.info };
    if (markdownInfo) {
      delete remainingInfo.markdown;
    }

    // Use problem workflow length if available, otherwise fallback to container count
    const totalSteps = problem?.workflow.length ?? submission.containers.length ?? 0;
    const progress = totalSteps > 0 ? ((submission.current_step + 1) / totalSteps) * 100 : 0;
    const canBeInterrupted = submission.status === 'Queued' || submission.status === 'Running';

    const handleInterrupt = async () => {
        if (!confirm(t('details.interrupt.confirm'))) return;
        try {
            await api.post(`/submissions/${submissionId}/interrupt`);
            toast({ title: t('details.interrupt.successTitle'), description: t('details.interrupt.successDescription') });
            mutate();
        } catch (err: any) {
            toast({ variant: 'destructive', title: t('details.interrupt.failTitle'), description: err.response?.data?.message || t('details.interrupt.failDefault') });
        }
    }

    return (
        <div className="grid gap-6 lg:grid-cols-3">
            <div className="lg:col-span-2">
                <Card>
                    <CardHeader>
                        <CardTitle>{t('details.log.title')}</CardTitle>
                        <CardDescription>{t('details.log.description')}</CardDescription>
                    </CardHeader>
                    <CardContent>
                        <SubmissionLogViewer submission={submission} problem={problem} onStatusUpdate={mutate} />
                    </CardContent>
                </Card>
            </div>
            
            <div className="space-y-6">
                 <Card>
                    <CardHeader>
                        <div className="flex items-center justify-between">
                            <CardTitle>{t('details.info.title')}</CardTitle>
                            {canBeInterrupted && (
                                <Button variant="destructive" size="sm" onClick={handleInterrupt}>
                                    <XCircle className="h-4 w-4 mr-1" /> {t('details.interrupt.button')}
                                </Button>
                            )}
                        </div>
                    </CardHeader>
                    <CardContent className="space-y-4 text-sm">
                        {/* --- Submission Details Section --- */}
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground flex items-center gap-2"><Hash className="h-4 w-4"/>{t('details.info.status')}</span>
                            <SubmissionStatusBadge status={submission.status} />
                        </div>
                        {submission.status === 'Queued' && <QueuePosition submissionId={submission.id} cluster={submission.cluster} />}
                        {(submission.status === 'Running') && totalSteps > 0 && (
                            <div>
                                <Progress value={progress} className="w-full" />
                                <p className="text-xs text-muted-foreground mt-1">{
                                problem?.workflow[submission.current_step]?.name ? 
                                t('details.info.stepProgress', {
                                    current: submission.current_step + 1,
                                    total: totalSteps,
                                    name: problem.workflow[submission.current_step].name
                                })
                                :
                                t('details.info.stepProgressNoName', {
                                    current: submission.current_step + 1,
                                    total: totalSteps,
                                })
                            }</p>
                            </div>
                        )}
                        {problem?.score.mode != "performance" && (
                          <div className="flex items-center justify-between">
                              <span className="text-muted-foreground flex items-center gap-2"><Tag className="h-4 w-4"/>{t('details.info.score')}</span>
                              <span className="font-mono text-lg">{submission.score}</span>
                          </div>
                        )}
                        {problem?.score.mode != "score" && (
                          <div className="flex items-center justify-between">
                            <span className="text-muted-foreground flex items-center gap-2">
                              <Rocket className="h-4 w-4" />
                                {t('details.info.performance')}
                            </span>
                            <span className="font-mono text-lg ">{submission.performance?.toFixed(2)}</span>
                          </div>
                        )}
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground flex items-center gap-2"><Clock className="h-4 w-4"/>{t('details.info.submitted')}</span>
                            <span>{formatDistanceToNow(new Date(submission.CreatedAt), { addSuffix: true, locale: locales[locale] || enUS })}</span>
                        </div>
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground flex items-center gap-2"><Code className="h-4 w-4"/>{t('details.info.problem')}</span>
                             <Link href={`/problems?id=${submission.problem_id}`} className="text-primary hover:underline">
                                {submission.problem_id}
                             </Link>
                        </div>
                         <div className="flex items-center justify-between">
                            <span className="text-muted-foreground flex items-center gap-2"><User className="h-4 w-4"/>{t('details.info.user')}</span>
                            <span>{submission.user.nickname}</span>
                        </div>
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground flex items-center gap-2"><Layers className="h-4 w-4"/>{t('details.info.cluster')}</span>
                            <span>{submission.cluster}</span>
                        </div>
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground flex items-center gap-2"><Server className="h-4 w-4"/>{t('details.info.node')}</span>
                            <span>{submission.node || 'N/A'}</span>
                        </div>
                        
                        {markdownInfo && (
                            <>
                                <Separator className="my-4" />
                                <div className="space-y-2">
                                    <MarkdownViewer content={markdownInfo as string} />
                                </div>
                            </>
                        )}

                        {remainingInfo && Object.keys(remainingInfo).length > 0 && (
                             <>
                                <Separator className="my-4" />
                                <div className="space-y-2">
                                    <h3 className="font-semibold tracking-tight">{t('details.judgeInfo.title')}</h3>
                                    <pre className="p-4 bg-muted rounded-md text-xs overflow-auto">
                                        {JSON.stringify(remainingInfo, null, 2)}
                                    </pre>
                                    <p className="text-xs text-muted-foreground">{t('details.judgeInfo.description')}</p>
                                </div>
                             </>
                        )}
                    </CardContent>
                 </Card>
            </div>
        </div>
    );
}


function SubmissionDetailsSkeleton() {
    const t = useTranslations('submissions');
    return (
      <div className="grid gap-6 lg:grid-cols-3">
          <div className="lg:col-span-2 space-y-6">
              <Card>
                  <CardHeader>
                      <Skeleton className="h-6 w-1/4" />
                      <Skeleton className="h-4 w-2/4" />
                  </CardHeader>
                  <CardContent>
                      <div className="bg-muted h-96 rounded-md p-4 space-y-2">
                           <Skeleton className="h-4 w-full" />
                           <Skeleton className="h-4 w-3/4" />
                      </div>
                  </CardContent>
              </Card>
          </div>
          <div className="space-y-6">
              <Card>
                  <CardHeader><CardTitle>{t('details.info.title')}</CardTitle></CardHeader>
                  <CardContent className="space-y-4">
                      {[...Array(6)].map((_, i) => (
                           <div key={i} className="flex justify-between">
                               <Skeleton className="h-5 w-1/3" />
                               <Skeleton className="h-5 w-1/2" />
                           </div>
                      ))}
                  </CardContent>
              </Card>
          </div>
      </div>
    );
}

// Main page component that decides which view to render
function SubmissionsPageContent() {
    const searchParams = useSearchParams();
    const submissionId = searchParams.get('id');

    if (submissionId) {
        return <SubmissionDetails submissionId={submissionId} />;
    }

    return <MySubmissionsList />;
}

export default function MySubmissionsPage() {
    return (
        <Suspense fallback={<SubmissionDetailsSkeleton />}>
            <SubmissionsPageContent />
        </Suspense>
    );
}