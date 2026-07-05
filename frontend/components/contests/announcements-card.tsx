"use client";

import useSWR from 'swr';
import api from '@/lib/api';
import { Announcement } from '@/lib/types';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Megaphone } from 'lucide-react';
import { format, formatDistanceToNow } from 'date-fns';
import { zhCN, enUS, Locale } from "date-fns/locale";
import { useLocale } from "next-intl";
import { Separator } from '../ui/separator';
import MarkdownViewer from '../shared/markdown-viewer';
import { useTranslations } from 'next-intl';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

export function AnnouncementsCard({ contestId }: { contestId: string }) {
    const locale = useLocale();
    const locales: Record<string, Locale> = {
        zh: zhCN,
        en: enUS,
    };
    const t = useTranslations('contests.announcements');
    
    const { data: announcements, error, isLoading } = useSWR<Announcement[]>(`/contests/${contestId}/announcements`, fetcher, {
        refreshInterval: 60000 // Refresh every minute
    });

    return (
        <Card>
            <CardHeader>
                <CardTitle className="flex items-center gap-2">
                    <Megaphone />
                    {t('title')}
                </CardTitle>
            </CardHeader>
            <CardContent>
                {isLoading && (
                    <div className="space-y-4">
                        <Skeleton className="h-4 w-3/4" />
                        <Skeleton className="h-4 w-1/2" />
                        <Separator />
                        <Skeleton className="h-4 w-3/4" />
                        <Skeleton className="h-4 w-1/2" />
                    </div>
                )}
                {error && <p className="text-sm text-destructive">{t('loadFail')}</p>}
                {!isLoading && announcements && announcements.length > 0 ? (
                    <div className="space-y-3">
                        {announcements.map((ann, index) => (
                            <div key={ann.id}>
                                <div className="space-y-1">
                                    <h3 className="font-semibold">{ann.title}</h3>
                                    <p className="text-xs text-muted-foreground" title={format(new Date(ann.created_at), 'Pp', { locale: locales[locale] || enUS })}>
                                        {formatDistanceToNow(new Date(ann.created_at), { addSuffix: true, locale: locales[locale] || enUS })}
                                    </p>
                                    <div>
                                        <MarkdownViewer content={ann.description} />
                                    </div>
                                </div>
                                {index < announcements.length - 1 && <Separator className="mt-6" />}
                            </div>
                        ))}
                    </div>
                ) : !isLoading && (
                    <p className="text-sm text-muted-foreground text-center py-4">{t('none')}</p>
                )}
            </CardContent>
        </Card>
    );
}