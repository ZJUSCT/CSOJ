"use client";
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2 } from 'lucide-react';

// This page acts as a redirect to the main contests page after login.
export default function DashboardPage() {
  const router = useRouter();

  useEffect(() => {
    router.replace('/contests');
  }, [router]);

  return (
    <div className="flex h-full flex-1 items-center justify-center">
      <Loader2 className="h-16 w-16 animate-spin text-primary" />
    </div>
  );
}