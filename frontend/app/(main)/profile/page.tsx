"use client";
import { useAuth } from '@/hooks/use-auth';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useToast } from '@/hooks/use-toast';
import api from '@/lib/api';
import { Skeleton } from '@/components/ui/skeleton';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { getInitials } from '@/lib/utils';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { TokenInfoCard } from '@/components/profile/token-info-card';
import { useTranslations } from 'next-intl'; // Import useTranslations
import { Badge } from '@/components/ui/badge';

export default function ProfilePage() {
    const t = useTranslations('Profile'); // Initialize translations
    const { user, isLoading, logout } = useAuth();
    const { toast } = useToast();
    const router = useRouter();
    const [isUploading, setIsUploading] = useState(false);

    // Schema definition must be inside the component or outside, but using t() inside requires it to be inside
    // or passed as a function argument, let's redefine it here to use t() for error messages.
    const profileSchema = z.object({
        nickname: z.string().min(1, t('form.nicknameRequired')).max(15),
        signature: z.string().max(100).optional(),
    });

    const form = useForm<z.infer<typeof profileSchema>>({
        resolver: zodResolver(profileSchema),
        values: {
            nickname: user?.nickname || '',
            signature: user?.signature || '',
        },
    });

    const handleAvatarUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (!file) return;

        const formData = new FormData();
        formData.append('avatar', file);
        setIsUploading(true);

        try {
            await api.post('/user/avatar', formData, {
                headers: { 'Content-Type': 'multipart/form-data' },
            });
            toast({ title: t('avatar.uploadSuccess') });
            // Forcing a reload to get the new user profile with updated avatar URL
            window.location.reload();
        } catch (error: any) {
            toast({
                variant: 'destructive',
                title: t('avatar.uploadFailTitle'),
                description: error.response?.data?.message || t('avatar.uploadFailDescription'),
            });
        } finally {
            setIsUploading(false);
        }
    };

    const onSubmit = async (values: z.infer<typeof profileSchema>) => {
        try {
            await api.patch('/user/profile', values);
            toast({ title: t('form.updateSuccess') });
            // Forcing a reload to get the new user profile
            window.location.reload();
        } catch (error: any) {
             toast({
                variant: 'destructive',
                title: t('form.updateFailTitle'),
                description: error.response?.data?.message || t('form.updateFailDescription'),
            });
        }
    };

    const handleLogout = () => {
        logout();
        router.push('/login');
    }

    if (isLoading || !user) {
        return <Skeleton className="w-full h-96" />;
    }

    const isSubmitting = form.formState.isSubmitting;

    return (
        <div className="grid auto-rows-min gap-6 lg:grid-cols-3">
            <Card>
                <CardHeader>
                    <CardTitle>{t('avatar.title')}</CardTitle>
                    <CardDescription>{t('avatar.description')}</CardDescription>
                </CardHeader>
                <CardContent className="flex flex-col items-center gap-4">
                    <Avatar className="h-32 w-32">
                        <AvatarImage src={user.avatar_url} alt={user.nickname} />
                        <AvatarFallback>{getInitials(user.nickname)}</AvatarFallback>
                    </Avatar>
                    <Input id="avatar-upload" type="file" accept="image/*" onChange={handleAvatarUpload} className="hidden" />
                    <Button asChild variant="outline">
                        <label htmlFor="avatar-upload">{isUploading ? t('avatar.uploading') : t('avatar.change')}</label>
                    </Button>
                </CardContent>
            </Card>

            <Card className="lg:col-span-2 lg:row-span-2 flex flex-col">
                <CardHeader>
                    <CardTitle>{t('form.title')}</CardTitle>
                    <CardDescription>{t('form.description')}</CardDescription>
                </CardHeader>
                <CardContent className="flex flex-1 flex-col justify-between">
                    <Form {...form}>
                        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                            <FormItem>
                                <FormLabel>{t('form.username')}</FormLabel>
                                <Input disabled value={user.username} />
                            </FormItem>
                            <FormField
                                control={form.control}
                                name="nickname"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>{t('form.nickname')}</FormLabel>
                                        <FormControl>
                                            <Input placeholder={t('form.nicknamePlaceholder')} {...field} />
                                        </FormControl>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={form.control}
                                name="signature"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>{t('form.signature')}</FormLabel>
                                        <FormControl>
                                            <Input placeholder={t('form.signaturePlaceholder')} {...field} />
                                        </FormControl>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                             <FormItem>
                                <FormLabel>{t('form.tags')}</FormLabel>
                                <div className="flex flex-wrap gap-1 mt-1">
                                    {user.tags && user.tags.split(',').map(tag => tag.trim() ? (
                                        <Badge key={tag} variant="secondary">{tag.trim()}</Badge>
                                    ) : null)}
                                    {!user.tags && <p className="text-sm text-muted-foreground">{t('form.noTags')}</p>}
                                </div>
                            </FormItem>

                            <Button type="submit" disabled={isSubmitting}>
                                {isSubmitting ? t('form.saving') : t('form.saveChanges')}
                            </Button>
                        </form>
                    </Form>
                    <div className="border-t pt-6 mt-6">
                        <Button variant="destructive" onClick={handleLogout}>{t('logout')}</Button>
                    </div>
                </CardContent>
            </Card>

            <TokenInfoCard />
        </div>
    );
}