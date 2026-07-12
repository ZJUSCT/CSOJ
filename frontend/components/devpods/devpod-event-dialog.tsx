"use client";

import { useState } from "react";
import { format } from "date-fns";
import { Loader2, ScrollText } from "lucide-react";
import useSWR from "swr";

import api from "@/lib/api";
import { DevPodEventListResponse } from "@/lib/types";
import { useTranslations } from "next-intl";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";

const fetcher = (url: string) => api.get(url).then(response => response.data.data);

function eventTime(timestamp: string) {
  if (!timestamp || timestamp.startsWith("0001-")) return "-";
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? "-" : format(date, "yyyy-MM-dd HH:mm:ss");
}

export function DevPodEventButton({ endpoint, name, disabled }: {
  endpoint: string;
  name: string;
  disabled?: boolean;
}) {
  const t = useTranslations("devpods");
  const [open, setOpen] = useState(false);
  const { data, error, isLoading, isValidating } = useSWR<DevPodEventListResponse>(
    open ? endpoint : null,
    fetcher,
    { refreshInterval: open ? 5000 : 0, keepPreviousData: true },
  );
  const events = data?.items ?? [];
  const warnings = data?.warnings ?? [];

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="ghost" className="w-24" disabled={disabled}>
          <ScrollText className="mr-1.5 h-3.5 w-3.5" />
          {t("events")}
        </Button>
      </DialogTrigger>
      <DialogContent aria-describedby={undefined} className="max-h-[calc(100dvh-2rem)] max-w-4xl overflow-hidden">
        <DialogHeader>
          <DialogTitle>{t("eventTitle")}: {name}</DialogTitle>
        </DialogHeader>
        <div className="max-h-[65dvh] space-y-2 overflow-y-auto pr-1">
          {isLoading && Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-24 w-full" />)}
          {error && (
            <div className="rounded-md border border-destructive/50 bg-destructive/5 p-4 text-sm text-destructive">
              {error.response?.data?.message ?? error.message ?? t("eventLoadFailed")}
            </div>
          )}
          {warnings.map((warning, index) => (
            <div key={`${warning}-${index}`} className="rounded-md border border-amber-500/50 bg-amber-500/5 p-3 text-sm text-amber-700 dark:text-amber-300">
              {warning}
            </div>
          ))}
          {!isLoading && !error && events.length === 0 && (
            <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
              {t("noEvents")}
            </div>
          )}
          {events.map(event => (
            <div key={event.name} className="rounded-md border p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <Badge variant={event.type === "Warning" ? "destructive" : "secondary"}>{event.type || "Normal"}</Badge>
                  <span className="font-medium">{event.reason || "Event"}</span>
                  {event.count > 1 && <Badge variant="outline">×{event.count}</Badge>}
                </div>
                <time className="whitespace-nowrap text-xs text-muted-foreground" dateTime={event.timestamp}>{eventTime(event.timestamp)}</time>
              </div>
              <p className="mt-2 whitespace-pre-wrap break-words text-sm">{event.message || "-"}</p>
              <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground">
                <span>{event.object_kind || "Object"}/{event.object_name}</span>
                {event.source && <span>{event.source}</span>}
              </div>
            </div>
          ))}
          {isValidating && !isLoading && <div className="flex justify-center py-1"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>}
        </div>
      </DialogContent>
    </Dialog>
  );
}
