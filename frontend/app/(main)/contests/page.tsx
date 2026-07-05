"use client";
import { useSearchParams } from 'next/navigation';
import { Suspense, useEffect, useMemo, useState } from 'react';
import useSWR, { useSWRConfig } from 'swr';
import { Contest, Problem, LeaderboardEntry, TrendEntry, ScoreHistoryPoint } from '@/lib/types';
import api from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { Skeleton } from '@/components/ui/skeleton';
import { format } from 'date-fns';
import { zhCN, enUS, Locale } from "date-fns/locale";
import { useLocale, useTranslations } from "next-intl";
import { Calendar, Clock, BookOpen, Trophy, CheckCircle, Edit3, Loader2, Swords, CheckCheck } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import MarkdownViewer from '@/components/shared/markdown-viewer';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card';
import { UserProfileCard } from '@/components/shared/user-profile-card';
import { getInitials, cn, getTagColorClasses } from '@/lib/utils';
import EchartsTrendChart from '@/components/charts/echarts-trend-chart';
import { AnnouncementsCard } from '@/components/contests/announcements-card';
import { DifficultyBadge } from '@/components/contests/difficulty-badge';
import UserScoreCard from '@/components/contests/user-score-card';
import { Label } from "@/components/ui/label";
import { Search, List } from 'lucide-react';
import { Badge } from '@/components/ui/badge';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

function ContestTimeline({ contest }: { contest: Contest }) {
    const t = useTranslations('contests');
    // Using a state for 'now' to make the component re-render and animate
    const [now, setNow] = useState(new Date().getTime());

    useEffect(() => {
        // Update 'now' every second for live progress
        const timer = setInterval(() => setNow(new Date().getTime()), 1000);
        return () => clearInterval(timer);
    }, []);

    // Time & State Calculation
    const startTime = new Date(contest.starttime).getTime();
    const endTime = new Date(contest.endtime).getTime();
    const totalDuration = endTime - startTime;

    if (totalDuration <= 0) {
        return <div className="text-center text-sm text-muted-foreground">{t('invalidDuration')}</div>;
    }

    const hasStarted = now >= startTime;
    const hasEnded = now > endTime;
    const status = hasEnded ? 'ended' : hasStarted ? 'ongoing' : 'upcoming';

    // Fixed Timeline Axis Calculation (always extends 10% on both sides)
    const marginRatio = 0.10;
    // Calculate the margin size so the contest duration takes up (1 - 2 * marginRatio) of the axis
    const margin = totalDuration * marginRatio / (1 - 2 * marginRatio);
    const axisStart = startTime - margin;
    const axisEnd = endTime + margin;

    // Position Calculation Helper
    const getPositionPercent = (time: number) => {
        if (axisEnd === axisStart) return 0; // Avoid division by zero
        const percent = ((time - axisStart) / (axisEnd - axisStart)) * 100;
        return Math.max(0, Math.min(100, percent));
    };

    const startPos = getPositionPercent(startTime);
    const endPos = getPositionPercent(endTime);
    const nowPos = getPositionPercent(now);

    // 4. Progress Bar Styling & Status Text
    const progressInContest = Math.max(0, Math.min(1, (now - startTime) / totalDuration));
    let barColor = '';
    let statusText = '';
    let statusColorClass = '';

    switch (status) {
        case 'upcoming':
            statusText = t('status.upcoming');
            statusColorClass = 'text-blue-500';
            barColor = 'rgb(59 130 246)'; // blue-500
            break;
        case 'ended':
            statusText = t('status.ended');
            statusColorClass = 'text-gray-500';
            barColor = 'rgb(156 163 175)'; // gray-400
            break;
        case 'ongoing':
            statusText = t('status.live');
            statusColorClass = 'text-green-500';
            // Hue transitions from 120 (green) -> 60 (yellow) -> 0 (red)
            const hue = 120 * (1 - progressInContest);
            barColor = `hsl(${hue}, 80%, 50%)`;
            break;
    }

    const barStyle: React.CSSProperties = {
        left: '0%',
        width: `${nowPos}%`,
        backgroundColor: barColor,
        transition: 'width 1s linear, background-color 1s linear'
    };

    return (
        <div className="pt-2 pb-2">
            <div className="relative h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                {/* The colored progress bar */}
                <div
                    className="h-full absolute"
                    style={barStyle}
                />

                {/* Start time vertical marker */}
                <div
                    className="absolute top-0 h-full w-0.5 bg-gray-500 opacity-75"
                    style={{ left: `${startPos}%` }}
                    title={`${t('starts')}: ${format(new Date(startTime), 'MMM d, HH:mm')}`}
                />

                {/* End time vertical marker */}
                <div
                    className="absolute top-0 h-full w-0.5 bg-gray-500 opacity-75"
                    style={{ left: `${endPos}%` }}
                    title={`${t('ends')}: ${format(new Date(endTime), 'MMM d, HH:mm')}`}
                />
            </div>

            <div className="flex justify-between mt-2 text-xs text-muted-foreground">
                <span>{format(new Date(startTime), 'yyyy/MM/dd HH:mm')}</span>
                <span>{format(new Date(endTime), 'yyyy/MM/dd HH:mm')}</span>
            </div>
        </div>
    );
}

function ContestCard({ contest }: { contest: Contest }) {
    const locale = useLocale();
    const locales: Record<string, Locale> = {
        zh: zhCN,
        en: enUS,
    };
    const t = useTranslations('contests');
    const { data: history, isLoading: isHistoryLoading } = useSWR<ScoreHistoryPoint[]>(`/contests/${contest.id}/history`, fetcher);
    const { mutate } = useSWRConfig();
    const { toast } = useToast();
    const [isRegistered, setIsRegistered] = useState(false);
    const [isRegistering, setIsRegistering] = useState(false);

    useEffect(() => {
        setIsRegistered(!!history && history.length > 0);
    }, [history]);

    const handleRegister = async (e: React.MouseEvent) => {
        e.preventDefault();
        e.stopPropagation();
        setIsRegistering(true);
        try {
            await api.post(`/contests/${contest.id}/register`);
            toast({ title: t('registration.successTitle'), description: t('registration.successDescription') });
            mutate(`/contests/${contest.id}/history`);
        } catch (error: any) {
            toast({ variant: "destructive", title: t('registration.failTitle'), description: error.response?.data?.message || t('registration.unexpectedError') });
        } finally {
            setIsRegistering(false);
        }
    };

    const now = new Date();
    const startTime = new Date(contest.starttime);
    const endTime = new Date(contest.endtime);
    const hasStarted = now >= startTime;
    const hasEnded = now > endTime;

    let statusText = t('status.upcoming');
    if (hasStarted && !hasEnded) statusText = t('status.ongoing');
    if (hasEnded) statusText = t('status.finished');

    const canRegister = statusText === t('status.ongoing');
    const isLoadingRegistration = isHistoryLoading || isRegistering;

    return (
        <Link href={`/contests?id=${contest.id}`} passHref>
            <Card className="hover:shadow-lg transition-shadow duration-300">
                <CardHeader>
                    <CardTitle className="text-xl">
                        <Link href={`/contests?id=${contest.id}`} passHref>
                            {contest.name}
                        </Link>
                    </CardTitle>
                    <CardDescription>
                        <Link href={`/contests?id=${contest.id}`} passHref>
                            <span className={`text-base font-bold ${statusText === t('status.ongoing') ? 'text-green-600' : statusText === t('status.finished') ? 'text-red-600' : 'text-blue-600'}`}>{statusText}</span>
                        </Link>
                    </CardDescription>
                </CardHeader>
                <CardContent className="space-y-4 text-sm text-muted-foreground">
                    <div className="space-y-2">
                        <div className="flex items-center gap-2"><Calendar className="h-4 w-4" /><span>{format(startTime, 'PPP', { locale: locales[locale] || enUS })} - {format(endTime, 'PPP', { locale: locales[locale] || enUS })}</span></div>
                        <div className="flex items-center gap-2"><Clock className="h-4 w-4" /><span>{format(startTime, 'HH:mm')} {t('to')} {format(endTime, 'HH:mm')}</span></div>
                    </div>
                    <ContestTimeline contest={contest} />
                </CardContent>
                <CardFooter className="flex">
                    {hasEnded ? (
                        <Button disabled className="ml-auto opacity-60 cursor-not-allowed">
                            <CheckCheck className="mr-2 h-4 w-4" /> {t('status.finished')}
                        </Button>
                    ) : !hasStarted ? (
                        <Button disabled className="ml-auto opacity-60 cursor-not-allowed">
                            <Calendar className="mr-2 h-4 w-4" /> {t('status.upcoming')}
                        </Button>
                    ) : (
                        isRegistered ? (
                            <Button disabled className="ml-auto">
                                <CheckCircle className="mr-2 h-4 w-4" /> {t('registered')}
                            </Button>
                        ) : (
                            <Button
                                onClick={(e) => {
                                    e.preventDefault();
                                    handleRegister(e);
                                }}
                                disabled={isLoadingRegistration}
                                className="ml-auto"
                            >
                                {isLoadingRegistration ? (
                                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                ) : (
                                    <Edit3 className="mr-2 h-4 w-4" />
                                )}
                                {isLoadingRegistration ? t('checking') : t('register')}
                            </Button>
                        )
                    )}
                </CardFooter>
            </Card>
        </Link>
    );
}

function ContestList() {
    const t = useTranslations('contests');
    const { data: contests, error, isLoading } = useSWR<Record<string, Contest>>('/contests', fetcher);

    if (isLoading) return (
        <div className="grid gap-6 grid-cols-1">
            {[...Array(3)].map((_, i) => (
                <Card key={i}>
                    <CardHeader><Skeleton className="h-6 w-3/4" /><Skeleton className="h-4 w-1/2" /></CardHeader>
                    <CardContent className="space-y-2"><Skeleton className="h-4 w-full" /><Skeleton className="h-4 w-5/6" /></CardContent>
                    <CardFooter><Skeleton className="h-10 w-24" /></CardFooter>
                </Card>
            ))}
        </div>
    );
    if (error) return <div>{t('list.loadFail')}</div>;
    if (!contests || Object.keys(contests).length === 0) return <div>{t('list.noContests')}</div>;

    return (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
            {Object.values(contests).map(contest => (
                <ContestCard key={contest.id} contest={contest} />
            ))}
        </div>
    );
}

function ProblemCard({ problemId, index }: { problemId: string; index: number }) {
    const t = useTranslations('contests');
    const { data: problem, isLoading } = useSWR<Problem>(`/problems/${problemId}`, fetcher);

    if (isLoading) return <Skeleton className="h-28 w-full" />;

    return (
        <Link href={`/problems?id=${problemId}`} className="relative block overflow-hidden rounded-lg hover:shadow-lg transition-shadow duration-300">
            <Card className="h-full">
                <div className="relative z-10">
                    <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                        <CardTitle className="font-bold">{problem?.name || problemId}</CardTitle>
                        <BookOpen className="h-4 w-4 text-muted-foreground" />
                    </CardHeader>
                    <CardContent>
                        <div className="flex items-center gap-2 mt-2">
                            {problem?.score?.mode === 'performance' && (
                                <Badge
                                    variant="flat"
                                    className="inline-flex items-center gap-2 rounded-full bg-gray-100 dark:bg-zinc-700 text-gray-800 dark:text-gray-200 px-3 py-1 text-sm font-medium select-none"
                                >
                                    <Swords className="w-3 h-3" />
                                    {t('problemCard.performance')}
                                </Badge>
                            )}

                            <DifficultyBadge level={problem?.level || ""} />
                        </div>

                    </CardContent>
                </div>

                <div
                    aria-hidden="true"
                    className="
                        absolute bottom-0 right-0 z-0
                        transform translate-x-1/5 translate-y-1/4
                        text-9xl font-extrabold
                        text-gray-100 dark:text-zinc-700
                        pointer-events-none select-none
                    "
                >
                    {index}
                </div>
            </Card>
        </Link>
    );
}

function ContestProblems({ contestId }: { contestId: string }) {
    const t = useTranslations('contests');
    const { data: contest, error, isLoading } = useSWR<Contest>(`/contests/${contestId}`, fetcher);

    if (isLoading) return <Skeleton className="h-64 w-full" />;
    if (error) return <div>{t('detail.loadFail')}</div>;
    if (!contest) return <div>{t('detail.notFound')}</div>;

    return (
        <div className="space-y-6">
            <Card>
                <CardHeader>
                    <div className="flex items-center space-x-2">
                        <Search className="w-5 h-5" />
                        <CardTitle className="font-bold">{t('description.title')}</CardTitle>
                    </div>
                </CardHeader>
                <CardContent>
                    <MarkdownViewer
                        content={contest.description}
                        assetContext="contest"
                        assetContextId={contest.id}
                    />
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <div className="flex items-center space-x-2">
                        <List className="w-5 h-5" />
                        <CardTitle className="font-bold">{t('problems.title')}</CardTitle>
                    </div>
                    <CardDescription>
                        {contest.problem_ids.length > 0
                            ? t('problems.instruction')
                            : t('problems.none')}
                    </CardDescription>
                </CardHeader>
                <CardContent className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
                    {contest.problem_ids.map((problemId, i) => (
                        <ProblemCard key={problemId} problemId={problemId} index={i + 1} />
                    ))}
                </CardContent>
            </Card>
        </div>
    );
}


function ContestTrend({ contest }: { contest: Contest }) {
    const t = useTranslations('contests');
    const { data: trendData, error, isLoading } = useSWR<TrendEntry[]>(`/contests/${contest.id}/trend`, fetcher, { refreshInterval: 30000 });

    if (isLoading) return <Skeleton className="h-[500px] w-full" />;
    if (error) return <div>{t('trend.loadFail')}</div>;
    if (!trendData || trendData.length === 0) {
        return (
            <Card>
                <CardHeader>
                    <CardTitle>{t('trend.title')}</CardTitle>
                    <CardDescription>{t('trend.description')}</CardDescription>
                </CardHeader>
                <CardContent className="h-96 w-full flex items-center justify-center">
                    <p className="text-muted-foreground">{t('trend.none')}</p>
                </CardContent>
            </Card>
        );
    }

    return (
        <Card>
            <CardHeader>
                <CardTitle>{t('trend.title')}</CardTitle>
                <CardDescription>{t('trend.description')}</CardDescription>
            </CardHeader>
            <CardContent className="h-[500px] w-full">
                <EchartsTrendChart
                    trendData={trendData}
                    contestStartTime={contest.starttime}
                    contestEndTime={contest.endtime}
                />
            </CardContent>
        </Card>
    );
}

function LeaderboardRow({ entry, rank, problemIds, isRankDisabled }: { entry: LeaderboardEntry, rank: number | string, problemIds: string[], isRankDisabled: boolean }) {
    const getRankColor = (rank: number | string) => {
        if (rank === 1) return 'text-yellow-400';
        if (rank === 2) return 'text-gray-400';
        if (rank === 3) return 'text-yellow-600';
        return '';
    };

    return (
        <TableRow key={entry.user_id} className={cn(isRankDisabled && "text-muted-foreground")}>
            <TableCell className={`font-medium text-lg ${getRankColor(rank)}`}>
                <div className="flex items-center gap-2">
                    {typeof rank === 'number' && rank <= 3 && <Trophy className="h-5 w-5" />}
                    {rank}
                </div>
            </TableCell>
            <TableCell>
                <HoverCard>
                    <HoverCardTrigger asChild>
                        <div className="flex items-center gap-3 cursor-pointer">
                        <Avatar className="h-8 w-8">
                            <AvatarImage src={entry.avatar_url} alt={entry.nickname} />
                            <AvatarFallback>{getInitials(entry.nickname)}</AvatarFallback>
                        </Avatar>
                        <div className="flex flex-row items-center flex-wrap gap-3">
                            <span className="font-medium">{entry.nickname}</span>
                            <div className="flex flex-wrap gap-1">
                                {entry.tags && entry.tags.split(',').map(tag => {
                                    const trimmedTag = tag.trim();
                                    if (!trimmedTag) return null;
                                    return (
                                        <Badge 
                                            key={trimmedTag}
                                            variant="flat" 
                                            className={cn(
                                                "text-xs border-transparent",
                                                getTagColorClasses(trimmedTag)
                                            )}
                                        >
                                            {trimmedTag}
                                        </Badge>
                                    );
                                })}
                            </div>
                        </div>
                    </div>
                    </HoverCardTrigger>
                    <HoverCardContent className="w-80">
                        <UserProfileCard userId={entry.user_id} />
                    </HoverCardContent>
                </HoverCard>
            </TableCell>
            {problemIds.map(problemId => (
                <TableCell key={problemId} className="text-center font-mono">
                    {entry.problem_scores[problemId] ?? '–'}
                </TableCell>
            ))}
            <TableCell className="text-right font-mono text-lg">{entry.total_score}</TableCell>
        </TableRow>
    );
}

function ContestLeaderboard({ contestId }: { contestId: string }) {
    const t = useTranslations('contests');
    // State for tag filtering
    const [selectedTags, setSelectedTags] = useState<string[]>([]);

    // Fetch contest details to get problem IDs
    const { data: contest, error: contestError, isLoading: isContestLoading } = useSWR<Contest>(`/contests/${contestId}`, fetcher);

    // Fetch unfiltered leaderboard only to derive available tags
    const { data: leaderboardUnfiltered, isLoading: isLoadingUnfiltered } = useSWR<LeaderboardEntry[]>(`/contests/${contestId}/leaderboard`, fetcher, { refreshInterval: 60000 }); // Slow refresh is fine

    const availableTags = useMemo(() => {
        if (!leaderboardUnfiltered) return [];
        const allTags = new Set<string>();
        leaderboardUnfiltered.forEach(entry => {
            if (entry.tags) {
                entry.tags.split(',').forEach(tag => {
                    const trimmedTag = tag.trim();
                    if (trimmedTag) allTags.add(trimmedTag);
                });
            }
        });
        return Array.from(allTags).sort().map(tag => ({ value: tag, label: tag }));
    }, [leaderboardUnfiltered]);

    // Toggle tag selection
    const toggleTag = (tagValue: string) => {
        setSelectedTags(prev =>
            prev.includes(tagValue)
                ? prev.filter(t => t !== tagValue)
                : [...prev, tagValue]
        );
    };

    // Fetch leaderboard data with the selected tags filter
    const tagsQuery = selectedTags.join(',');
    const { data: leaderboardData, error: leaderboardError, isLoading: isLeaderboardLoading } = useSWR<LeaderboardEntry[]>(`/contests/${contestId}/leaderboard?tags=${tagsQuery}`, fetcher, { refreshInterval: 15000 });
    const leaderboard = leaderboardData ?? [];

    const isLoading = isContestLoading || isLeaderboardLoading;
    if (isLoading) return <Skeleton className="h-64 w-full" />;
    if (contestError || leaderboardError) return <div>{t('leaderboard.loadFail')}</div>;
    if (leaderboard.length === 0 && selectedTags.length === 0) return <div>{t('leaderboard.none')}</div>;
    if (!contest) return <div>{t('leaderboard.contestDetailsFail')}</div>;

    const problemIds = contest.problem_ids;

    let rankToDisplay = 0;
    let realRankCounter = 0;
    let previousScore = -Infinity; // Use a value that is guaranteed to be less than any possible score

    return (
        <Card>
            <CardHeader>
                <CardTitle>{t('leaderboard.title')}</CardTitle>
            </CardHeader>
            <CardContent>
                <div className="mb-4 flex items-center gap-2">
                    <Label className="shrink-0 font-semibold">{t('leaderboard.filterTags')}:</Label>
                     {isLoadingUnfiltered ? <Skeleton className="h-6 w-32" /> : (
                         <div className="flex flex-wrap gap-2">
                             {availableTags.length === 0 && <span className="text-sm text-muted-foreground">{t('leaderboard.noTags')}</span>}
                             {availableTags.map(tag => (
                                 <Badge
                                     key={tag.value}
                                     variant={selectedTags.includes(tag.value) ? "default" : "outline"}
                                     onClick={() => toggleTag(tag.value)}
                                     className="cursor-pointer"
                                 >
                                     {tag.label}
                                 </Badge>
                             ))}
                         </div>
                     )}
                </div>
                <Table>
                    <TableHeader>
                        <TableRow>
                            <TableHead className="w-[80px]">{t('leaderboard.rank')}</TableHead>
                            <TableHead>{t('leaderboard.user')}</TableHead>
                            {problemIds.map((id, index) => (
                                <TableHead key={id} className="text-center">
                                    <Link href={`/problems?id=${id}`} className="hover:underline" title={id}>
                                        P{index + 1}
                                    </Link>
                                </TableHead>
                            ))}
                            <TableHead className="text-right">{t('leaderboard.totalScore')}</TableHead>
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {leaderboard.map((entry) => {
                            const isRankDisabled = entry.disable_rank;
                            let displayRank: number | string = '-';

                            if (!isRankDisabled) {
                                realRankCounter++;
                                // Update rank only when the score is different from the previous entry's score
                                if (entry.total_score !== previousScore) {
                                    rankToDisplay = realRankCounter;
                                }
                                displayRank = rankToDisplay;
                                previousScore = entry.total_score;
                            }

                            return (
                                <LeaderboardRow
                                    key={entry.user_id}
                                    entry={entry}
                                    rank={displayRank}
                                    problemIds={problemIds}
                                    isRankDisabled={isRankDisabled} />
                            );
                        })}
                    </TableBody>
                </Table>
            </CardContent>
        </Card>
    );
}

function ContestDetailView({ contestId, view }: { contestId: string, view: string }) {
    const t = useTranslations('contests');
    const { data: contest, isLoading: isContestLoading } = useSWR<Contest>(`/contests/${contestId}`, fetcher);
    const { data: history, isLoading: isHistoryLoading } = useSWR<ScoreHistoryPoint[]>(`/contests/${contestId}/history`, fetcher);
    const { mutate } = useSWRConfig();
    const { toast } = useToast();
    const [isRegistered, setIsRegistered] = useState(false);

    useEffect(() => {
        if (history && history.length > 0) {
            setIsRegistered(true);
        } else if (history) {
            setIsRegistered(false);
        }
    }, [history]);

    const handleRegister = async () => {
        try {
            await api.post(`/contests/${contestId}/register`);
            toast({ title: t('registration.successTitle'), description: t('registration.successDescription') });
            mutate(`/contests/${contestId}/history`);
        } catch (error: any) {
            toast({ variant: "destructive", title: t('registration.failTitle'), description: error.response?.data?.message || t('registration.unexpectedError') });
        }
    };

    const now = new Date();
    const canRegister = contest && now >= new Date(contest.starttime) && now <= new Date(contest.endtime);

    if (isContestLoading) {
        return (
            <div className="space-y-6">
                <Skeleton className="h-10 w-1/2" />
                <div className="grid gap-8 lg:grid-cols-4 items-start">
                    <div className="lg:col-span-3 space-y-6">
                        <Skeleton className="h-12 w-full" />
                        <Skeleton className="h-96 w-full" />
                    </div>
                    <div className="space-y-6 lg:sticky lg:top-20">
                        <Skeleton className="h-48 w-full" />
                    </div>
                </div>
            </div>
        );
    }

    if (!contest) {
        return <div>{t('detail.notFound')}</div>;
    }

    return (
        <div className="space-y-6">
            <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
                <h1 className="text-3xl font-bold">{contest.name}</h1>
                {canRegister && (
                    isRegistered ? (
                        <Button disabled variant="secondary">
                            <CheckCircle className="mr-2 h-4 w-4" /> {t('registered')}
                        </Button>
                    ) : (
                        <Button onClick={handleRegister} disabled={isHistoryLoading}>
                            {isHistoryLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Edit3 className="mr-2 h-4 w-4" />}
                            {isHistoryLoading ? t('loading') : t('registerForContest')}
                        </Button>
                    )
                )}
            </div>

            <div className="grid gap-8 lg:grid-cols-4 items-start">
                <div className="lg:col-span-3 space-y-6">
                    <Tabs value={view} className="w-full">
                        <TabsList className="grid w-full grid-cols-2">
                            <TabsTrigger value="problems" asChild>
                                <Link href={`/contests?id=${contestId}&view=problems`}>{t('tabs.problems')}</Link>
                            </TabsTrigger>
                            <TabsTrigger value="leaderboard" asChild>
                                <Link href={`/contests?id=${contestId}&view=leaderboard`}>{t('tabs.leaderboard')}</Link>
                            </TabsTrigger>
                        </TabsList>
                    </Tabs>
                    <div>
                        {view === 'leaderboard' ? (
                            <div className="space-y-6">
                                <ContestTrend contest={contest} />
                                <ContestLeaderboard contestId={contestId} />
                            </div>
                        ) : (
                            <ContestProblems contestId={contestId} />
                        )}
                    </div>
                </div>

                <div className="space-y-6 lg:sticky lg:top-20">
                     <UserScoreCard contestId={contestId} />
                     <AnnouncementsCard contestId={contestId} />
                </div>
            </div>
        </div>
    );
}


function ContestsPageContent() {
    const searchParams = useSearchParams();
    const contestId = searchParams.get('id');
    const view = searchParams.get('view') || 'problems';

    if (contestId) {
        return <ContestDetailView contestId={contestId} view={view} />;
    }

    return <ContestList />;
}

export default function ContestsPage() {
    return (
        <Suspense fallback={<Skeleton className="w-full h-96" />}>
            <ContestsPageContent />
        </Suspense>
    );
}