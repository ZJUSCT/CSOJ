"use client";
import { useState, useEffect, useMemo } from 'react';
import { useAuth } from '@/hooks/use-auth';
import { Problem, Submission } from '@/lib/types';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs';
import { useTranslations } from 'next-intl';
import { TerminalLogViewer } from '@/components/shared/terminal-log-viewer';

// --- Main Orchestrator Component ---
interface SubmissionLogViewerProps {
    submission: Submission;
    problem?: Problem; // Make problem optional
    onStatusUpdate: () => void;
}

export function SubmissionLogViewer({ submission, problem, onStatusUpdate }: SubmissionLogViewerProps) {
    const t = useTranslations('submissions.logViewer.mainViewer');
    const { token } = useAuth();
    const [selectedContainerId, setSelectedContainerId] = useState<string | null>(null);

    // If problem info is available, use its workflow. Otherwise, create a default workflow
    // based on the number of containers, assuming all logs are visible.
    const workflow = useMemo(() =>
        (problem?.workflow ?? submission.containers.map((_, index) => ({
            name: `Step ${index + 1}`,
            show: true,
        }))) as { name: string; show: boolean }[],
    [problem, submission.containers]);

    useEffect(() => {
        // Automatically select the last container, which is usually the active or most recent one.
        if (submission.containers.length > 0) {
            const lastContainer = submission.containers[submission.containers.length - 1];
            if(selectedContainerId !== lastContainer.id) {
                setSelectedContainerId(lastContainer.id);
            }
        }
    // Only re-run when the number of containers changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [submission.containers.length]);

    if (submission.containers.length === 0) {
        return (
            <div className="font-mono text-xs bg-muted rounded-md h-[60vh] overflow-y-auto p-4 text-muted-foreground flex items-center justify-center">
                {t('queueMessage')}
            </div>
        );
    }

    const getWsUrl = (containerId: string | null) => {
        if (!token || !containerId || typeof window === 'undefined') return null;

        const containerIndex = submission.containers.findIndex(c => c.id === containerId);
        if (containerIndex === -1 || !workflow[containerIndex]?.show) {
            return null;
        }

        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const host = window.location.host;
        return `${wsProtocol}//${host}/api/v1/ws/submissions/${submission.id}/containers/${containerId}/logs?token=${token}`;
    };

    return (
        <Tabs value={selectedContainerId ?? ""} onValueChange={setSelectedContainerId} className="w-full">
            <TabsList className="grid h-auto w-full gap-1" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
                {submission.containers.map((container, index) => (
                    <TabsTrigger
                        key={container.id}
                        value={container.id}
                        disabled={!workflow[index]?.show}
                    >
                        {t('tabLabel', { step: index + 1, name: workflow[index]?.name || '' }) + (workflow[index]?.show ? '' : t('tabHidden'))}
                    </TabsTrigger>
                ))}
            </TabsList>
            {submission.containers.map((container, index) => {
                const isRunning = container.status === 'Running';
                const canShow = workflow[index]?.show;

                return (
                    <TabsContent key={container.id} value={container.id} className="mt-4">
                        {!canShow ? (
                            <div className="font-mono text-xs bg-muted rounded-md h-[60vh] overflow-y-auto p-4 text-muted-foreground flex items-center justify-center">
                                {t('hiddenLogMessage')}
                            </div>
                        ) : isRunning ? (
                            <TerminalLogViewer mode="realtime" wsUrl={getWsUrl(container.id)} onStatusUpdate={onStatusUpdate} />
                        ) : (
                            <TerminalLogViewer mode="static" staticLogUrl={`/submissions/${submission.id}/containers/${container.id}/log`} />
                        )}
                    </TabsContent>
                )
            })}
        </Tabs>
    );
}
