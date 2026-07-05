"use client";

import { useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { Loader2 } from 'lucide-react';

function AuthCallbackContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { login } = useAuth();
  const { toast } = useToast();

  useEffect(() => {
    const token = searchParams.get('token');
    const error = searchParams.get('error');

    if (error) {
      toast({
        variant: 'destructive',
        title: 'Authentication Failed',
        description: `Could not log in with GitLab. Reason: ${error.replace(/_/g, ' ')}`,
      });
      router.replace('/login');
    } else if (token) {
      login(token);
      toast({
        title: 'Login Successful!',
        description: 'Welcome back.',
      });
      router.replace('/contests');
    } else {
      // This case should not be reached in a normal flow
      toast({
        variant: 'destructive',
        title: 'Invalid Callback URL',
        description: 'No token or error found in the URL.',
      });
      router.replace('/login');
    }
  }, [router, searchParams, login, toast]);

  return (
    <div className="flex h-screen flex-col items-center justify-center gap-4">
      <Loader2 className="h-12 w-12 animate-spin text-primary" />
      <p className="text-muted-foreground">Authenticating, please wait...</p>
    </div>
  );
}

export default function AuthCallbackPage() {
    return (
        <Suspense fallback={
            <div className="flex h-screen flex-col items-center justify-center gap-4">
                <Loader2 className="h-12 w-12 animate-spin text-primary" />
                <p className="text-muted-foreground">Loading...</p>
            </div>
        }>
            <AuthCallbackContent />
        </Suspense>
    );
}