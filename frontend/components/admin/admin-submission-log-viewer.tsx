"use client";
import { useState, useEffect, useMemo } from 'react';
import { Problem, Submission } from '@/lib/types';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card';
import { TerminalLogViewer } from '@/components/shared/terminal-log-viewer';

export function AdminSubmissionLogViewer({ submission, problem, onStatusUpdate }: { submission: Submission, problem?: Problem, onStatusUpdate: () => void }) {
    const [selectedContainerId, setSelectedContainerId] = useState<string | null>(null);

    // If problem info is available, use its workflow. Otherwise, create a default workflow
    // based on the number of containers.
    const workflow = useMemo(() =>
        (problem?.workflow ?? submission.containers.map((_, index) => ({
            name: `Step ${index + 1}`,
            show: true, // Admins can always see logs
        }))) as { name: string; show: boolean }[],
    [problem, submission.containers]);

    useEffect(() => {
        if (submission?.containers && submission.containers.length > 0) {
            const lastContainer = submission.containers[submission.containers.length - 1];
            if (selectedContainerId !== lastContainer.id) {
                setSelectedContainerId(lastContainer.id);
            }
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [submission?.containers.length]);

    const getWsUrl = (containerId: string | null) => {
        if (!containerId || typeof window === 'undefined') return null;
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const host = window.location.host;
        return `${wsProtocol}//${host}/api/v1/admin/ws/submissions/${submission.id}/containers/${containerId}/logs`;
    };

    if (submission.containers.length === 0) {
        return (
            <Card className="flex flex-col h-full">
                <CardHeader>
                    <CardTitle>Live Log</CardTitle>
                    <CardDescription>Real-time output from the judge containers.</CardDescription>
                </CardHeader>
                <CardContent className="flex-1">
                    <div className="font-mono text-xs bg-muted rounded-md h-[60vh] overflow-y-auto p-4 text-muted-foreground flex items-center justify-center">
                        Submission is in queue. No logs to display yet.
                    </div>
                </CardContent>
            </Card>
        );
    }

    return (
        <Card className="flex flex-col h-full">
            <CardHeader><CardTitle>Live Log</CardTitle><CardDescription>Real-time output from the judge containers.</CardDescription></CardHeader>
            <CardContent className="flex flex-col flex-1">
                <Tabs value={selectedContainerId ?? ""} onValueChange={setSelectedContainerId} className="w-full flex flex-col flex-1">
                    <TabsList className="grid h-auto w-full gap-1" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))' }}>
                        {submission.containers.map((c, i) => (
                            <TabsTrigger key={c.id} value={c.id}>
                                {workflow[i]?.name || `Step ${i + 1}`}
                            </TabsTrigger>
                        ))}
                    </TabsList>
                    {submission.containers.map(c => (
                        <TabsContent key={c.id} value={c.id} className="mt-4 flex-1">
                            {c.status === 'Running' ?
                                <TerminalLogViewer mode="realtime" wsUrl={getWsUrl(c.id)} onStatusUpdate={onStatusUpdate} /> :
                                <TerminalLogViewer mode="static" staticLogUrl={`/admin/submissions/${submission.id}/containers/${c.id}/log`} />
                            }
                        </TabsContent>
                    ))}
                </Tabs>
            </CardContent>
        </Card>
    );
}
