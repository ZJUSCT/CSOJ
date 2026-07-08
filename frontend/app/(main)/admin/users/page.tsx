"use client"
import { Suspense, useState, useEffect } from "react"
import { useSearchParams } from "next/navigation"
import useSWR from 'swr';
import api from '@/lib/api';
import { User, UserBestScore, ScoreHistoryPoint, Contest } from '@/lib/types';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { MoreHorizontal, History } from "lucide-react"
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { CreateUserDialog, DeleteUserMenuItem, EditUserDialog, RegisterContestDialog, ResetPasswordDialog } from "@/components/admin/user-actions";
import { Avatar, AvatarImage, AvatarFallback } from "@/components/ui/avatar";
import { getInitials } from "@/lib/utils";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { format } from "date-fns";
import Link from "next/link";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import withAdmin from "@/components/layout/with-admin";
import { AdminSubNav } from "@/components/layout/admin-sub-nav";
import { useAuth } from "@/hooks/use-auth";

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

// A simple debounce hook
function useDebounce(value: string, delay: number) {
    const [debouncedValue, setDebouncedValue] = useState(value);
    useEffect(() => {
        const handler = setTimeout(() => {
            setDebouncedValue(value);
        }, delay);
        return () => {
            clearTimeout(handler);
        };
    }, [value, delay]);
    return debouncedValue;
}

function UserList() {
    const [searchQuery, setSearchQuery] = useState('');
    const debouncedSearchQuery = useDebounce(searchQuery, 300);
    const { data: users, error, isLoading, mutate } = useSWR<User[]>(`/admin/users?query=${debouncedSearchQuery}`, fetcher);
    const { user: currentUser } = useAuth();
    const isSuperAdmin = currentUser?.role === "superadmin";

    const handleRoleToggle = async (u: User) => {
        const newRole = u.role === "admin" ? "user" : "admin";
        try {
            await api.patch(`/admin/users/${u.id}/role`, { role: newRole });
            mutate();
        } catch (err: any) {
            // SWR onError toast surfaces the error
        }
    };

    if (error) return <div>Failed to load users.</div>;

    return (
        <Card>
            <CardHeader>
                <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
                    <div>
                        <CardTitle>Users</CardTitle>
                        <CardDescription>Manage all user accounts.</CardDescription>
                    </div>
                    <div className="flex gap-2 w-full md:w-auto">
                        <Input
                            placeholder="Search"
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            className="w-full md:w-64"
                        />
                        <CreateUserDialog onUserCreated={mutate} />
                    </div>
                </div>
            </CardHeader>
            <CardContent>
                {isLoading ? <Skeleton className="h-64 w-full" /> :
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead>ID</TableHead>
                                <TableHead>Username</TableHead>
                                <TableHead>Nickname</TableHead>
                                <TableHead>Tags</TableHead>
                                <TableHead className="text-right">Actions</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {users?.map(user => (
                                <TableRow key={user.id}>
                                    <TableCell><Link href={`/admin/users?id=${user.id}`} className="font-mono text-primary hover:underline">{user.id}</Link></TableCell>
                                    <TableCell className="flex items-center gap-2">
                                        {user.username}
                                        {user.banned_until && new Date(user.banned_until) > new Date() && (
                                            <Badge variant="destructive">Banned</Badge>
                                        )}
                                        {user.disable_rank && (
                                            <Badge variant="secondary">Unranked</Badge>
                                        )}
                                    </TableCell>
                                    <TableCell>{user.nickname}</TableCell>
                                    <TableCell>
                                        {user.tags && user.tags.split(',').map(tag => tag.trim() ? (
                                            <Badge key={tag} variant="secondary" className="mr-1 mb-1">
                                                {tag.trim()}
                                            </Badge>
                                        ) : null)}
                                    </TableCell>
                                    <TableCell className="text-right">
                                        <DropdownMenu>
                                            <DropdownMenuTrigger asChild>
                                                <Button variant="ghost" className="h-8 w-8 p-0">
                                                    <span className="sr-only">Open menu</span>
                                                    <MoreHorizontal className="h-4 w-4" />
                                                </Button>
                                            </DropdownMenuTrigger>
                                            <DropdownMenuContent align="end">
                                                <DropdownMenuLabel>Actions</DropdownMenuLabel>
                                                <EditUserDialog
                                                    user={user}
                                                    onUserUpdated={mutate}
                                                    trigger={<DropdownMenuItem onSelect={(e) => e.preventDefault()}>Edit Profile</DropdownMenuItem>}
                                                />
                                                <ResetPasswordDialog
                                                    userId={user.id}
                                                    trigger={<DropdownMenuItem onSelect={(e) => e.preventDefault()}>Reset Password</DropdownMenuItem>}
                                                />
                                                <RegisterContestDialog
                                                    userId={user.id}
                                                    trigger={<DropdownMenuItem onSelect={(e) => e.preventDefault()}>Register for Contest</DropdownMenuItem>}
                                                />
                                                {isSuperAdmin && (
                                                    <DropdownMenuItem
                                                        onSelect={(e) => {
                                                            e.preventDefault();
                                                            handleRoleToggle(user);
                                                        }}
                                                    >
                                                        {user.role === "admin"
                                                            ? "Demote to User"
                                                            : "Promote to Admin"}
                                                    </DropdownMenuItem>
                                                )}
                                                <DropdownMenuSeparator />
                                                <DeleteUserMenuItem userId={user.id} onUserDeleted={mutate} />
                                            </DropdownMenuContent>
                                        </DropdownMenu>
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                }
            </CardContent>
        </Card>
    );
}

function UserContestHistory({ userId }: { userId: string }) {
    const [selectedContestId, setSelectedContestId] = useState<string>();
    const { data: contestsData, isLoading: contestsLoading } = useSWR<Record<string, Contest>>('/admin/contests', fetcher);
    const { data: history, isLoading: historyLoading } = useSWR<ScoreHistoryPoint[]>(
        selectedContestId ? `/admin/users/${userId}/history?contest_id=${selectedContestId}` : null,
        fetcher
    );

    const contests = contestsData ? Object.values(contestsData) : [];

    return (
        <Card>
            <CardHeader>
                <CardTitle>Contest Score History</CardTitle>
                <CardDescription>View a user's score progression over time for a specific contest.</CardDescription>
            </CardHeader>
            <CardContent>
                <div className="mb-4">
                    <Select onValueChange={setSelectedContestId} value={selectedContestId}>
                        <SelectTrigger className="w-full md:w-[280px]">
                            <SelectValue placeholder={contestsLoading ? "Loading contests..." : "Select a contest"} />
                        </SelectTrigger>
                        <SelectContent>
                            {contests.map(contest => (
                                <SelectItem key={contest.id} value={contest.id}>
                                    {contest.name} ({contest.id})
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>
                {(historyLoading && selectedContestId) && <Skeleton className="h-48 w-full" />}
                {history && (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead>Time</TableHead>
                                <TableHead>Triggering Problem</TableHead>
                                <TableHead>Total Score After Change</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {history.map((h, i) => (
                                <TableRow key={i}>
                                    <TableCell>{format(new Date(h.time), 'Pp')}</TableCell>
                                    <TableCell>{h.problem_id ? <Link href={`/admin/problems?id=${h.problem_id}`} className="text-primary hover:underline">{h.problem_id}</Link> : <span className="text-muted-foreground">Registration</span>}</TableCell>
                                    <TableCell>{h.score}</TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                )}
                {history?.length === 0 && selectedContestId && <p className="text-muted-foreground text-center p-4">No history found for this contest.</p>}
            </CardContent>
        </Card>
    );
}


function UserDetails({ userId }: { userId: string }) {
    const { data: user, isLoading: userLoading } = useSWR<User>(`/admin/users/${userId}`, fetcher);
    const { data: scores, isLoading: scoresLoading } = useSWR<UserBestScore[]>(`/admin/users/${userId}/scores`, fetcher);

    if (userLoading) return <Skeleton className="h-96 w-full" />;

    return (
        <div className="space-y-6">
            <Card>
                <CardHeader className="flex flex-row items-start gap-4">
                    <Avatar className="h-16 w-16">
                        <AvatarImage src={user?.avatar_url} alt={user?.nickname} />
                        <AvatarFallback>{user ? getInitials(user.nickname) : '?'}</AvatarFallback>
                    </Avatar>
                    <div className="flex-1">
                        <div className="flex items-center justify-between">
                             <div>
                                <CardTitle className="text-2xl">{user?.nickname}</CardTitle>
                                <CardDescription>@{user?.username} ({user?.id})</CardDescription>
                             </div>
                             <div className="flex items-center gap-2">
                                {user?.banned_until && new Date(user.banned_until) > new Date() && (
                                    <Badge variant="destructive" className="text-base">BANNED</Badge>
                                )}
                                {user?.disable_rank && (
                                    <Badge variant="secondary" className="text-base">UNRANKED</Badge>
                                )}
                             </div>
                        </div>
                        {user?.banned_until && new Date(user.banned_until) > new Date() && (
                             <div className="mt-2 border-l-4 border-destructive pl-4">
                                 <p className="text-sm font-semibold">Ban Reason: <span className="font-normal">{user.ban_reason || "No reason provided."}</span></p>
                                 <p className="text-sm font-semibold">Banned Until: <span className="font-normal">{format(new Date(user.banned_until), 'Pp')}</span></p>
                             </div>
                         )}
                        {user?.tags && (
                            <div className="mt-2 flex flex-wrap gap-1">
                                {user.tags.split(',').map(tag => tag.trim() ? (
                                    <Badge key={tag} variant="outline">{tag.trim()}</Badge>
                                ) : null)}
                            </div>
                        )}
                    </div>
                </CardHeader>
            </Card>
            <Tabs defaultValue="scores">
                <TabsList className="grid w-full grid-cols-2">
                    <TabsTrigger value="scores">Best Scores</TabsTrigger>
                    <TabsTrigger value="history">Contest History</TabsTrigger>
                </TabsList>
                <TabsContent value="scores" className="mt-4">
                    <Card>
                        <CardHeader><CardTitle>Best Scores by Problem</CardTitle></CardHeader>
                        <CardContent>
                            {scoresLoading ? <Skeleton className="h-48 w-full" /> :
                                <Table>
                                    <TableHeader>
                                        <TableRow>
                                            <TableHead>Problem ID</TableHead>
                                            <TableHead>Contest ID</TableHead>
                                            <TableHead>Score</TableHead>
                                            <TableHead>Submission ID</TableHead>
                                            <TableHead>Attempts</TableHead>
                                        </TableRow>
                                    </TableHeader>
                                    <TableBody>
                                        {scores?.map(s => (
                                            <TableRow key={s.ID}>
                                                <TableCell><Link href={`/admin/problems?id=${s.ProblemID}`} className="text-primary hover:underline">{s.ProblemID}</Link></TableCell>
                                                <TableCell><Link href={`/admin/contests?id=${s.ContestID}`} className="text-primary hover:underline">{s.ContestID}</Link></TableCell>
                                                <TableCell>{s.Score}</TableCell>
                                                <TableCell><Link href={`/admin/submissions?id=${s.SubmissionID}`} className="font-mono text-primary hover:underline">{s.SubmissionID.substring(0, 8)}...</Link></TableCell>
                                                <TableCell>{s.SubmissionCount}</TableCell>
                                            </TableRow>
                                        ))}
                                    </TableBody>
                                </Table>
                            }
                        </CardContent>
                    </Card>
                </TabsContent>
                <TabsContent value="history" className="mt-4">
                    <UserContestHistory userId={userId} />
                </TabsContent>
            </Tabs>
        </div>
    )
}


function UsersPageContent() {
    const searchParams = useSearchParams()
    const userId = searchParams.get('id')

    return (
        <div className="flex min-h-[calc(100vh-3.5rem)]">
            <AdminSubNav />
            <div className="flex-1 p-6 space-y-6 overflow-auto">
            {userId ? <UserDetails userId={userId} /> : <UserList />}
            </div>
        </div>
    );
}

function UsersPage() {
    return (
        <Suspense fallback={<Skeleton className="h-screen w-full" />}>
            <UsersPageContent />
        </Suspense>
    );
}

export default withAdmin(UsersPage);
