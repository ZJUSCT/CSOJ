"use client";

import React, { createContext, useState, useEffect, useCallback } from 'react';
import { jwtDecode, JwtPayload } from 'jwt-decode';
import { useSWRConfig } from 'swr';
import { User } from '@/lib/types';
import api from '@/lib/api';
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { format } from 'date-fns';
import { useTranslations } from 'next-intl';
import { Ban } from 'lucide-react';

interface AuthState {
  token: string | null;
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
}

export interface AuthContextType extends AuthState {
  login: (token: string) => void;
  logout: () => void;
}

export const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const t = useTranslations('banAlert');
  const [authState, setAuthState] = useState<AuthState>({
    token: null,
    user: null,
    isAuthenticated: false,
    isLoading: true,
  });
  const [banInfo, setBanInfo] = useState<{ reason: string; until: string } | null>(null);
  const { mutate } = useSWRConfig();

  const fetchUserProfile = useCallback(async (token: string) => {
    try {
      const response = await api.get('/user/profile', {
        headers: { Authorization: `Bearer ${token}` },
      });
      setAuthState({
        token,
        user: response.data.data,
        isAuthenticated: true,
        isLoading: false,
      });
    } catch (error) {
      console.error('Failed to fetch user profile, logging out.', error);
      const errorResponse = (error as any).response;
      if (!errorResponse || (errorResponse.status !== 403 || !errorResponse.data?.data?.ban_reason)) {
        logout();
      }
    }
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem('csoj_jwt');
    setAuthState({
      token: null,
      user: null,
      isAuthenticated: false,
      isLoading: false,
    });
    // Clear all SWR cache on logout
    mutate(() => true, undefined, { revalidate: false });
  }, [mutate]);

  useEffect(() => {
    const handleBan = (event: Event) => {
      const customEvent = event as CustomEvent;
      const { reason, until } = customEvent.detail;
      logout();
      setBanInfo({ reason, until });
    };

    window.addEventListener('userBanned', handleBan);

    return () => {
      window.removeEventListener('userBanned', handleBan);
    };
  }, [logout]);


  useEffect(() => {
    const token = localStorage.getItem('csoj_jwt');
    if (token) {
      try {
        const decoded = jwtDecode<JwtPayload>(token);
        if (decoded.exp! * 1000 > Date.now()) {
          fetchUserProfile(token);
        } else {
          logout();
        }
      } catch (e) {
        console.error("Invalid token", e);
        logout();
      }
    } else {
      setAuthState(prev => ({ ...prev, isLoading: false }));
    }
  }, [fetchUserProfile, logout]);

  const login = (token: string) => {
    localStorage.setItem('csoj_jwt', token);
    fetchUserProfile(token);
  };

  return (
    <AuthContext.Provider value={{ ...authState, login, logout }}>
      {children}
      {banInfo && (
        <AlertDialog open={!!banInfo}>
          <AlertDialogContent className="bg-destructive text-destructive-foreground border-destructive-foreground/20">
            <AlertDialogHeader>
              <AlertDialogTitle className="flex items-center gap-3 text-2xl font-bold">
                <Ban className="w-8 h-8 text-destructive-foreground" />
                {t('title')}
              </AlertDialogTitle>
              <AlertDialogDescription asChild>
                <div className="text-white text-lg pt-4 space-y-4">
                  <div>
                    <p className="font-semibold">{t('reasonLabel')}</p>
                    <p className="font-normal">{banInfo.reason}</p>
                  </div>
                  <div>
                    <p className="font-semibold">{t('untilLabel')}</p>
                    <p className="font-normal font-mono">
                      {format(new Date(banInfo.until), "yyyy-MM-dd HH:mm:ss")}
                    </p>
                  </div>
                </div>
              </AlertDialogDescription>
            </AlertDialogHeader>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </AuthContext.Provider>
  );
};