"use client";

import { useState, useEffect } from "react";
import { jwtDecode, JwtPayload } from "jwt-decode";
import { useAuth } from "@/hooks/use-auth";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CopyButton } from "@/components/ui/shadcn-io/copy-button";
import { useToast } from "@/hooks/use-toast";
import { Timer } from "lucide-react";
import { format, formatDistanceToNowStrict } from "date-fns";
import { zhCN, enUS, Locale } from "date-fns/locale";
import { useLocale } from "next-intl";
import { useTranslations } from "next-intl";

interface DecodedToken extends JwtPayload {}

export function TokenInfoCard() {
    const locale = useLocale();
    const locales: Record<string, Locale> = {
        zh: zhCN,
        en: enUS,
    };
    const t = useTranslations('Profile');
    const { token } = useAuth();
    const { toast } = useToast();
    const [decodedToken, setDecodedToken] = useState<DecodedToken | null>(null);
    const [expiresIn, setExpiresIn] = useState<string>("");

    useEffect(() => {
        if (token) {
            try {
                const decoded = jwtDecode<DecodedToken>(token);
                setDecodedToken(decoded);
            } catch (error) {
                console.error("Failed to decode token:", error);
                setDecodedToken(null);
            }
        }
    }, [token]);

    useEffect(() => {
        if (decodedToken?.exp) {
            const expiryDate = new Date(decodedToken.exp * 1000);
            
            const updateCountdown = () => {
                if (expiryDate > new Date()) {
                    setExpiresIn(formatDistanceToNowStrict(expiryDate, {locale: locales[locale] || enUS }));
                } else {
                    setExpiresIn(t('token.expired'));
                }
            };

            updateCountdown();
            const intervalId = setInterval(updateCountdown, 1000);

            return () => clearInterval(intervalId);
        }
    }, [decodedToken]);

    if (!token || !decodedToken) {
        return null;
    }

    const expiryDate = decodedToken.exp ? new Date(decodedToken.exp * 1000) : null;

    return (
        <Card>
            <CardHeader>
                <CardTitle>{t('token.title')}</CardTitle>
                <CardDescription>

                </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
                <div className="space-y-2">
                    <Label htmlFor="jwt-token">{t('token.label')}</Label>
                    <div className="flex items-center gap-2">
                        <Input id="jwt-token" readOnly value={token} className="min-w-0 flex-1 truncate font-mono text-xs" />
                        <CopyButton
                            content={token}
                            variant="outline"
                            className="h-9 w-9"
                            aria-label={t('token.copySr')}
                            onCopy={() => toast({
                                title: t('token.copySuccessTitle'),
                                description: t('token.copySuccessDescription'),
                            })}
                        />
                    </div>
                </div>

                {expiryDate && (
                    <div className="space-y-2 text-sm">
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground">{t('token.expiresAt')}</span>
                            <span>{format(expiryDate, "yyyy-MM-dd HH:mm:ss")}</span>
                        </div>
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground flex items-center gap-2">
                                <Timer className="h-4 w-4" />
                                {t('token.timeRemaining')}
                            </span>
                            <span className={`font-medium ${expiresIn === t('token.expired') ? "text-destructive" : ""}`}>{expiresIn}</span>
                        </div>
                    </div>
                )}
            </CardContent>
        </Card>
    );
}
