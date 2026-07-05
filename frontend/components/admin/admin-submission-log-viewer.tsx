"use client";
import { useState, useEffect, useRef, useMemo } from 'react';
import useWebSocket, { ReadyState } from 'react-use-websocket';
import { Problem, Submission } from '@/lib/types';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs';
import useSWR from 'swr';
import api from '@/lib/api';
import { Skeleton } from '../ui/skeleton';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card';

interface LogMessage {
    stream: 'stdout' | 'stderr' | 'info' | 'error';
    data: string;
}

const StaticLogViewer = ({ submissionId, containerId }: { submissionId: string, containerId: string }) => {
    const textFetcher = (url: string) => api.get(url, { responseType: 'text' }).then(res => res.data);
    const { data: logText, error, isLoading } = useSWR(`/admin/submissions/${submissionId}/containers/${containerId}/log`, textFetcher);
    const logContainerRef = useRef<HTMLDivElement>(null);

    const messages: LogMessage[] = useMemo(() => {
        if (!logText) return [];
        return logText.split('\n').filter(Boolean).map((line: string) => {
            try { return JSON.parse(line); } catch { return { stream: 'stdout', data: line }; }
        });
    }, [logText]);

    useEffect(() => { 
        if (logContainerRef.current) {
            logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight;
        }
    }, [messages]);

    return (
        <div className="relative h-full">
             <div className="absolute top-2 right-6 text-xs font-semibold flex items-center gap-2 z-10">
                <span className="h-2 w-2 rounded-full bg-gray-400"></span>
                Finished
            </div>
            <div ref={logContainerRef} className="font-mono text-xs bg-muted rounded-md h-[60vh] overflow-y-auto p-4">
                {isLoading && <Skeleton className="h-[60vh] w-full" />}
                {error && <p className="text-red-400">Failed to load log.</p>}
                {messages.map((msg, index) => (
                    <span key={index} className="whitespace-pre-wrap break-all">
                        {msg.stream === 'stderr' || msg.stream === 'error' ? (
                            <span className="text-red-400">{msg.data}</span>
                        ) : msg.stream === 'info' ? (
                            <span className="text-blue-400">{msg.data}</span>
                        ) : (
                            <span className="text-foreground">{msg.data}</span>
                        )}
                    </span>
                ))}
            </div>
        </div>
    );
};

const RealtimeLogViewer = ({ wsUrl, onStatusUpdate }: { wsUrl: string | null, onStatusUpdate: () => void }) => {
    const [messages, setMessages] = useState<LogMessage[]>([]);
    const logContainerRef = useRef<HTMLDivElement>(null);

    const { readyState, lastMessage } = useWebSocket(wsUrl, {
        shouldReconnect: (closeEvent) => closeEvent.code !== 1000,
        reconnectInterval: 3000,
        onClose: () => {
            console.log('WebSocket closed. Refetching submission status.');
            onStatusUpdate();
        }
    });

    useEffect(() => { setMessages([]); }, [wsUrl]);

    useEffect(() => {
        if (lastMessage?.data) { 
            try { 
                setMessages(prev => [...prev, JSON.parse(lastMessage.data)]); 
            } catch (e) {
                console.error("Failed to parse WebSocket message:", lastMessage.data);
            } 
        }
    }, [lastMessage]);

    useEffect(() => { 
        if (logContainerRef.current) {
            logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight;
        }
    }, [messages]);

    const connectionStatus = {
        [ReadyState.CONNECTING]: { text: 'Connecting...', color: 'bg-yellow-500' },
        [ReadyState.OPEN]: { text: 'Live', color: 'bg-green-500 animate-pulse' },
        [ReadyState.CLOSING]: { text: 'Closing...', color: 'bg-yellow-500' },
        [ReadyState.CLOSED]: { text: 'Disconnected', color: 'bg-red-500' },
        [ReadyState.UNINSTANTIATED]: { text: 'Idle', color: 'bg-gray-500' },
    }[readyState];

    return (
        <div className="relative h-full">
            <div className="absolute top-2 right-6 text-xs font-semibold flex items-center gap-2 z-10">
                <span className={`h-2 w-2 rounded-full ${connectionStatus.color}`}></span>
                {connectionStatus.text}
            </div>
            <div ref={logContainerRef} className="font-mono text-xs bg-muted rounded-md h-[60vh] overflow-y-auto p-4">
                {messages.length === 0 && readyState === ReadyState.OPEN && <p className="text-muted-foreground">Waiting for judge output...</p>}
                {messages.map((msg, index) => (
                    <span key={index} className="whitespace-pre-wrap break-all">
                        {msg.stream === 'stderr' || msg.stream === 'error' ? (
                            <span className="text-red-400">{msg.data}</span>
                        ) : msg.stream === 'info' ? (
                            <span className="text-blue-400">{msg.data}</span>
                        ) : (
                            <span className="text-foreground">{msg.data}</span>
                        )}
                    </span>
                ))}
            </div>
        </div>
    );
};

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
                                <RealtimeLogViewer wsUrl={getWsUrl(c.id)} onStatusUpdate={onStatusUpdate} /> :
                                <StaticLogViewer submissionId={submission.id} containerId={c.id} />
                            }
                        </TabsContent>
                    ))}
                </Tabs>
            </CardContent>
        </Card>
    );
}