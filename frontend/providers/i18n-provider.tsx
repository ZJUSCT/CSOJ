"use client";

import React, { useState, useEffect, useCallback } from 'react';
import { NextIntlClientProvider } from 'next-intl';
import { Loader2 } from 'lucide-react';

const AVAILABLE_LOCALES = ['en', 'zh'];
const DEFAULT_LOCALE = 'zh';
const LOCALE_STORAGE_KEY = 'csoj_locale';

interface ClientIntlProviderProps {
  children: React.ReactNode;
}

interface LocaleContextType {
    switchLocale: (newLocale: string) => void;
    locale: string | null;
}

const LocaleContext = React.createContext<LocaleContextType>({
    switchLocale: () => { 
        console.warn("switchLocale called outside ClientIntlProvider."); 
    },
    locale: null
});

export const useClientLocale = () => {
    const context = React.useContext(LocaleContext);
    if (!context) {
        throw new Error('useClientLocale must be used within ClientIntlProvider');
    }
    return context;
};

export const ClientIntlProvider: React.FC<ClientIntlProviderProps> = ({ children }) => {
  const [locale, setLocale] = useState<string | null>(null);
  const [messages, setMessages] = useState<Record<string, string> | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // Use a ref to track if a language load is in progress, preventing jittering from concurrent requests.
  const loadingRef = React.useRef(false); 

  const loadMessages = useCallback(async (targetLocale: string, updateStorage: boolean = true) => {
    // Avoid concurrent calls
    if (loadingRef.current) return false; 
    
    loadingRef.current = true;
    setIsLoading(true);

    try {
      // Check if the target language is available
      const finalLocale = AVAILABLE_LOCALES.includes(targetLocale) ? targetLocale : DEFAULT_LOCALE;
      
      const response = await fetch(`/messages/${finalLocale}.json`);
      if (!response.ok) {
        throw new Error(`Failed to load messages for locale: ${finalLocale}`);
      }
      const newMessages = await response.json();
      
      // Update state upon success
      setMessages(newMessages);
      setLocale(finalLocale);

      // Update local storage if requested
      if (updateStorage && typeof window !== 'undefined') {
        localStorage.setItem(LOCALE_STORAGE_KEY, finalLocale);
      }
      return true;
      
    } catch (error) {
      console.error(`Failed to load locale: ${targetLocale}.`, error);
      // On failure, only set default locale/messages if no locale has been loaded yet (initial load failure)
      if (!locale) {
          setMessages({});
          setLocale(DEFAULT_LOCALE);
      }
      return false;
    } finally {
      setIsLoading(false);
      loadingRef.current = false;
    }
  }, [locale]); 


  // Load initial locale on mount
  useEffect(() => {
    let initialLocale = DEFAULT_LOCALE;
    if (typeof window !== 'undefined') {
      const savedLocale = localStorage.getItem(LOCALE_STORAGE_KEY);
      if (savedLocale && AVAILABLE_LOCALES.includes(savedLocale)) {
        initialLocale = savedLocale;
      }
    }
    
    // Asynchronously load the initial language
    loadMessages(initialLocale);
    
  }, [loadMessages]);

  // Handle locale switching
  const switchLocale = async (newLocale: string) => {
    if (!AVAILABLE_LOCALES.includes(newLocale) || newLocale === locale || loadingRef.current) {
      return; 
    }
    
    // Attempt to load the new language pack
    const success = await loadMessages(newLocale, true); // Update storage on successful load
    
    if (!success) {
        console.warn(`Could not load ${newLocale}. Sticking to current locale.`);
    }
  };
  
  // Show full-screen loader if initial load is pending
  if (!locale || !messages) {
    if (isLoading) {
        return (
          <div className="flex h-screen flex-col items-center justify-center gap-4">
              <Loader2 className="h-12 w-12 animate-spin text-primary" />
          </div>
        );
    }
  }

  return (
    <LocaleContext.Provider value={{ switchLocale, locale }}>
        <NextIntlClientProvider locale={locale!} messages={messages!}>
            {children}
        </NextIntlClientProvider>
    </LocaleContext.Provider>
  );
};