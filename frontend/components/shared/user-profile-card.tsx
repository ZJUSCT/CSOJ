"use client";

import useSWR from 'swr';
import api from '@/lib/api';
import { User } from '@/lib/types';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { getInitials } from '@/lib/utils';
import { Skeleton } from '@/components/ui/skeleton';
import { Edit3 } from 'lucide-react';
import { Badge } from '../ui/badge';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

interface UserProfileCardProps {
    userId: string;
}

export function UserProfileCard({ userId }: UserProfileCardProps) {
    const { data: user, error, isLoading } = useSWR<User>(`/users/${userId}`, fetcher);

    if (isLoading) return <UserProfileCardSkeleton />;
    if (error || !user) return <div className="text-sm text-destructive">Could not load user profile.</div>;

    return (
        <div className="flex flex-col gap-4">
            <div className="flex items-center gap-4">
                <Avatar className="h-16 w-16">
                    <AvatarImage src={user.avatar_url} alt={user.nickname} />
                    <AvatarFallback>{getInitials(user.nickname)}</AvatarFallback>
                </Avatar>
                <div className="flex flex-col">
                    <h4 className="text-lg font-semibold">{user.nickname}</h4>
                    <p className="text-sm text-muted-foreground">@{user.username}</p>
                </div>
            </div>
            {user.signature && (
                <div className="text-sm text-foreground flex items-start gap-2">
                    <Edit3 className="h-4 w-4 mt-1 text-muted-foreground shrink-0" />
                    <p className="italic break-words whitespace-pre-wrap">{user.signature}</p>
                </div>
            )}
            {user.tags && (
                 <div className="flex flex-wrap gap-1">
                     {user.tags.split(',').map(tag => tag.trim() ? (
                         <Badge key={tag} variant="secondary">{tag.trim()}</Badge>
                     ) : null)}
                 </div>
            )}
        </div>
    );
}

export function UserProfileCardSkeleton() {
    return (
        <div className="flex flex-col gap-4">
            <div className="flex items-center gap-4">
                <Skeleton className="h-16 w-16 rounded-full" />
                <div className="space-y-2">
                    <Skeleton className="h-5 w-24" />
                    <Skeleton className="h-4 w-16" />
                </div>
            </div>
            <Skeleton className="h-4 w-full" />
        </div>
    );
}