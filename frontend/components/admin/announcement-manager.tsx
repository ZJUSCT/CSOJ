"use client";

import useSWR from 'swr';
import api from '@/lib/api';
import { Announcement } from '@/lib/types';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { format } from 'date-fns';
import { PlusCircle, Edit, Trash2 } from 'lucide-react';
import { AnnouncementFormDialog, DeleteAnnouncementButton } from './announcement-actions';
import { Separator } from '../ui/separator';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

export function AnnouncementManager({ contestId }: { contestId: string }) {
    const { data: announcements, error, isLoading, mutate } = useSWR<Announcement[]>(`/admin/contests/${contestId}/announcements`, fetcher);

    if (error) return <p>Failed to load announcements.</p>

    return (
        <Card>
            <CardHeader className="flex flex-row items-center justify-between">
                <div>
                    <CardTitle>Announcements</CardTitle>
                    <CardDescription>Manage contest announcements. These are visible to users after the contest starts.</CardDescription>
                </div>
                <AnnouncementFormDialog contestId={contestId} onSuccess={mutate} trigger={<Button><PlusCircle /> New Announcement</Button>} />
            </CardHeader>
            <CardContent>
                {isLoading ? <Skeleton className="h-48 w-full" /> :
                    <div className="space-y-6">
                        {announcements && announcements.length > 0 ? announcements.map((ann, index) => (
                            <div key={ann.id}>
                                <div className="flex items-start justify-between gap-4">
                                    <div className="flex-1 space-y-1">
                                        <h3 className="font-semibold">{ann.title}</h3>
                                        <p className="text-sm text-muted-foreground">
                                            Created: {format(new Date(ann.created_at), 'Pp')} | Updated: {format(new Date(ann.updated_at), 'Pp')}
                                        </p>
                                        <div className="prose prose-sm dark:prose-invert max-w-none pt-2" dangerouslySetInnerHTML={{ __html: ann.description.replace(/\n/g, '<br />') }} />
                                    </div>
                                    <div className="flex items-center">
                                        <AnnouncementFormDialog
                                            contestId={contestId}
                                            announcement={ann}
                                            onSuccess={mutate}
                                            trigger={<Button variant="ghost" size="icon"><Edit /></Button>}
                                        />
                                        <DeleteAnnouncementButton
                                            contestId={contestId}
                                            announcement={ann}
                                            onSuccess={mutate}
                                            trigger={<Button variant="ghost" size="icon" className="text-destructive hover:text-destructive"><Trash2 /></Button>}
                                        />
                                    </div>
                                </div>
                                {index < announcements.length - 1 && <Separator className="mt-6" />}
                            </div>
                        )) : (
                            <div className="text-center py-10 text-muted-foreground">
                                <p>No announcements have been posted yet.</p>
                            </div>
                        )}
                    </div>
                }
            </CardContent>
        </Card>
    );
}