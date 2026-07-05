"use client";
import { SWRConfig } from 'swr';

export const SWRProvider = ({ children }: { children: React.ReactNode }) => {
    return (
        <SWRConfig
            value={{
                revalidateOnFocus: true,
                errorRetryCount: 3,
            }}
        >
            {children}
        </SWRConfig>
    );
};