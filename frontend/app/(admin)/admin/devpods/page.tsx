"use client";

import useSWR from "swr";
import { format } from "date-fns";
import { Loader2, Play, RefreshCw, Square, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";

import api from "@/lib/api";
import { AdminDevPodInstance, AdminDevPodListResponse } from "@/lib/types";
import { useToast } from "@/hooks/use-toast";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

const fetcher = (url: string) => api.get(url).then(response => response.data.data);

function phaseVariant(phase: string): "default" | "secondary" | "destructive" | "outline" {
  switch (phase) {
    case "Running": return "default";
    case "Failed": return "destructive";
    case "Stopped": return "secondary";
    default: return "outline";
  }
}

function countPhase(items: AdminDevPodInstance[], phase: string) {
  return items.filter(item => item.phase === phase).length;
}

type DevPodAction = "start" | "stop" | "delete";

function devPodKey(item: Pick<AdminDevPodInstance, "cluster_name" | "name">) {
  return `${item.cluster_name}/${item.name}`;
}

export default function AdminDevPodsPage() {
  const { toast } = useToast();
  const [pendingActions, setPendingActions] = useState<Record<string, DevPodAction>>({});
  const { data, isLoading, isValidating, mutate } = useSWR<AdminDevPodListResponse>(
    "/admin/devpods",
    fetcher,
    { refreshInterval: 5000 },
  );

  useEffect(() => {
    if (!data) return;
    setPendingActions(previous => {
      const next = { ...previous };
      let changed = false;
      for (const [key, action] of Object.entries(previous)) {
        const item = data.items.find(candidate => devPodKey(candidate) === key);
        const completed = action === "delete"
          ? !item
          : action === "start"
            ? item?.phase === "Running" || item?.phase === "Failed"
            : item?.phase === "Stopped" || item?.phase === "Failed";
        if (completed) {
          delete next[key];
          changed = true;
        }
      }
      return changed ? next : previous;
    });
  }, [data]);

  const act = async (item: AdminDevPodInstance, action: DevPodAction) => {
    if (action === "delete" && !confirm(`Delete DevPod "${item.name}" from cluster "${item.cluster_name}"?`)) return;

    const key = devPodKey(item);
    setPendingActions(previous => ({ ...previous, [key]: action }));
    const baseURL = `/admin/devpods/${encodeURIComponent(item.cluster_name)}/${encodeURIComponent(item.name)}`;
    try {
      if (action === "delete") {
        await api.delete(baseURL);
      } else {
        await api.post(`${baseURL}/${action}`);
      }
      toast({ title: action === "delete" ? "DevPod deleted" : `DevPod ${action === "start" ? "starting" : "stopping"}` });
      void mutate();
    } catch (error: any) {
      toast({
        variant: "destructive",
        title: `Failed to ${action} DevPod`,
        description: error.response?.data?.message ?? error.message,
      });
      setPendingActions(previous => {
        const next = { ...previous };
        delete next[key];
        return next;
      });
    }
  };

  if (isLoading || !data) return <Skeleton className="h-96 w-full" />;

  const items = data.items ?? [];
  const summary = [
    { label: "Total", value: items.length },
    { label: "Running", value: countPhase(items, "Running") },
    { label: "Pending", value: countPhase(items, "Pending") },
    { label: "Stopped", value: countPhase(items, "Stopped") },
    { label: "Failed", value: countPhase(items, "Failed") },
  ];

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
        {summary.map(item => (
          <Card key={item.label}>
            <CardContent className="p-4">
              <div className="text-2xl font-semibold">{item.value}</div>
              <div className="text-xs text-muted-foreground">{item.label}</div>
            </CardContent>
          </Card>
        ))}
      </div>

      {data.warnings.length > 0 && (
        <Card className="border-yellow-500/50 bg-yellow-500/5">
          <CardContent className="p-4 text-sm text-yellow-700 dark:text-yellow-300">
            {data.warnings.map(warning => <div key={warning}>{warning}</div>)}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <CardTitle>DevPods</CardTitle>
          <Button variant="outline" size="sm" onClick={() => mutate()} disabled={isValidating}>
            <RefreshCw className={`mr-2 h-4 w-4 ${isValidating ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        </CardHeader>
        <CardContent>
          {items.length === 0 ? (
            <p className="text-sm text-muted-foreground">No DevPods found in the loaded clusters.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Owner</TableHead>
                  <TableHead>Template</TableHead>
                  <TableHead>Cluster</TableHead>
                  <TableHead>Desired</TableHead>
                  <TableHead>Phase</TableHead>
                  <TableHead>Endpoint</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map(item => {
                  const pendingAction = pendingActions[devPodKey(item)];
                  const isPending = !!pendingAction;
                  const showStop = pendingAction === "stop" || (pendingAction !== "start" && item.running);
                  return (
                    <TableRow key={`${item.cluster_name}/${item.namespace}/${item.name}`} aria-busy={isPending}>
                      <TableCell className="font-mono text-xs">{item.name}</TableCell>
                      <TableCell className="font-mono text-xs">{item.owner || "-"}</TableCell>
                      <TableCell>{item.template || "-"}</TableCell>
                      <TableCell>{item.cluster_name}</TableCell>
                      <TableCell><Badge variant={item.running ? "default" : "secondary"}>{item.running ? "Running" : "Stopped"}</Badge></TableCell>
                      <TableCell><Badge variant={phaseVariant(item.phase)}>{item.phase || "Unknown"}</Badge></TableCell>
                      <TableCell className="font-mono text-xs">{item.endpoint || "-"}</TableCell>
                      <TableCell className="max-w-48 truncate text-xs text-muted-foreground" title={item.message}>{item.message || "-"}</TableCell>
                      <TableCell className="whitespace-nowrap text-xs">{item.created_at ? format(new Date(item.created_at), "yyyy-MM-dd HH:mm:ss") : "-"}</TableCell>
                      <TableCell>
                        <div className="flex min-w-52 items-center justify-end gap-1">
                          {showStop ? (
                            <Button size="sm" variant="outline" className="w-24" disabled={isPending} onClick={() => act(item, "stop")}>
                              {pendingAction === "stop" ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Square className="mr-1.5 h-3.5 w-3.5" />}
                              {pendingAction === "stop" ? "Stopping" : "Stop"}
                            </Button>
                          ) : (
                            <Button size="sm" variant="outline" className="w-24" disabled={isPending} onClick={() => act(item, "start")}>
                              {pendingAction === "start" ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Play className="mr-1.5 h-3.5 w-3.5" />}
                              {pendingAction === "start" ? "Starting" : "Start"}
                            </Button>
                          )}
                          <Button size="sm" variant="ghost" className="w-24 text-destructive hover:text-destructive" disabled={isPending} onClick={() => act(item, "delete")}>
                            {pendingAction === "delete" ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Trash2 className="mr-1.5 h-3.5 w-3.5" />}
                            {pendingAction === "delete" ? "Deleting" : "Delete"}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
