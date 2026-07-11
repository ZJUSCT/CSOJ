"use client";
import useSWR from "swr";
import api from "@/lib/api";
import { DevPodListResponse, DevPodTemplate } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableHead, TableHeader, TableRow, TableCell } from "@/components/ui/table";
import { useTranslations } from "next-intl";
import { useToast } from "@/hooks/use-toast";
import { DevPodCard } from "@/components/devpods/devpod-card";
import { DevPodRow, DevPodAction } from "@/components/devpods/devpod-row";
import { useEffect, useState } from "react";

const fetcher = (url: string) => api.get(url).then(r => r.data.data);

export default function DevPodsPage() {
  const t = useTranslations("devpods");
  const { toast } = useToast();
  const { data: list, mutate } = useSWR<DevPodListResponse>("/devpods", fetcher, { refreshInterval: 5000 });
  const { data: templates } = useSWR<DevPodTemplate[]>("/devpods/templates", fetcher);
  const [busy, setBusy] = useState<string | null>(null);
  const [pendingActions, setPendingActions] = useState<Record<string, DevPodAction>>({});

  const items = list?.items ?? [];
  const maxPerUser = list?.max_per_user ?? 0;
  const runningCount = items.filter(item => item.running).length;
  const globalAtLimit = maxPerUser > 0 && runningCount >= maxPerUser;
  const usedByTpl = (id: string) => items.filter(i => i.template === id).length;
  const templateNames = new Map((templates ?? []).map(template => [template.id, template.name]));

  useEffect(() => {
    if (!list) return;
    setPendingActions(previous => {
      const next = { ...previous };
      let changed = false;
      for (const [name, action] of Object.entries(previous)) {
        const item = list.items.find(inst => inst.name === name);
        const completed = action === "delete"
          ? !item
          : action === "start"
            ? item?.phase === "Running" || item?.phase === "Failed"
            : item?.phase === "Stopped" || item?.phase === "Failed";
        if (completed) {
          delete next[name];
          changed = true;
        }
      }
      return changed ? next : previous;
    });
  }, [list]);

  const create = async (tplId: string) => {
    setBusy(tplId);
    try {
      await api.post("/devpods", { template_id: tplId });
      toast({ title: t("created") });
      mutate();
    } catch (e: any) {
      toast({ variant: "destructive", title: t("createFailed"), description: e.response?.data?.message });
    } finally {
      setBusy(null);
    }
  };

  const act = async (name: string, op: DevPodAction) => {
    if (op === "delete" && !confirm(t("deleteConfirm"))) return;
    setPendingActions(previous => ({ ...previous, [name]: op }));
    try {
      const url = op === "delete" ? `/devpods/${name}` : `/devpods/${name}/${op}`;
      await api[op === "delete" ? "delete" : "post"](url);
      await mutate();
    } catch (e: any) {
      setPendingActions(previous => {
        const next = { ...previous };
        delete next[name];
        return next;
      });
      toast({ variant: "destructive", title: e.response?.data?.message || "error" });
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold mb-4">{t("title")}</h2>
        <div className="grid gap-4 md:grid-cols-3">
          {(templates ?? []).map(tpl => (
            <DevPodCard key={tpl.id} tpl={tpl} used={usedByTpl(tpl.id)} disabled={busy === tpl.id} globalAtLimit={globalAtLimit} onCreate={() => create(tpl.id)} />
          ))}
          {templates && templates.length === 0 && (
            <Card><CardContent className="p-4 text-sm text-muted-foreground">No templates configured.</CardContent></Card>
          )}
        </div>
      </div>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between gap-4">
            <span>{t("myDevPods")}</span>
            {maxPerUser > 0 && <span className="text-sm font-normal text-muted-foreground">{t("globalUsage", { used: runningCount, max: maxPerUser })}</span>}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {items.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("noInstances")}</p>
          ) : (
            <Table>
              <TableHeader><TableRow>
                <TableHead>{t("name")}</TableHead>
                <TableHead>{t("template")}</TableHead>
                <TableHead>{t("status")}</TableHead>
                <TableHead>{t("sshCommand")}</TableHead>
                <TableHead>{t("createdAt")}</TableHead>
                <TableHead></TableHead>
              </TableRow></TableHeader>
              <TableBody>
                {items.map(inst => (
                  <DevPodRow key={inst.name} inst={inst} templateName={templateNames.get(inst.template)} pendingAction={pendingActions[inst.name]} startDisabled={globalAtLimit}
                    onStart={() => act(inst.name, "start")}
                    onStop={() => act(inst.name, "stop")}
                    onDelete={() => act(inst.name, "delete")} />
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
