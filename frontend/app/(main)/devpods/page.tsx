"use client";
import useSWR from "swr";
import api from "@/lib/api";
import { DevPodListResponse, DevPodTemplate, DevPodInstance } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableHead, TableHeader, TableRow, TableCell } from "@/components/ui/table";
import { useTranslations } from "next-intl";
import { useToast } from "@/hooks/use-toast";
import { DevPodCard } from "@/components/devpods/devpod-card";
import { DevPodRow } from "@/components/devpods/devpod-row";
import { useState } from "react";

const fetcher = (url: string) => api.get(url).then(r => r.data.data);

export default function DevPodsPage() {
  const t = useTranslations("devpods");
  const { toast } = useToast();
  const { data: list, mutate } = useSWR<DevPodListResponse>("/devpods", fetcher, { refreshInterval: 5000 });
  const { data: templates } = useSWR<DevPodTemplate[]>("/admin/devpod-templates", fetcher);
  const [busy, setBusy] = useState<string | null>(null);

  const items = list?.items ?? [];
  const usedByTpl = (id: string) => items.filter(i => i.template === id).length;

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

  const act = async (name: string, op: "start"|"stop"|"delete") => {
    if (op === "delete") {
      await api.delete(`/devpods/${name}`);
    } else {
      await api.post(`/devpods/${name}/${op}`);
    }
    mutate();
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold mb-4">{t("title")}</h2>
        <div className="grid gap-4 md:grid-cols-3">
          {(templates ?? []).map(tpl => (
            <DevPodCard key={tpl.id} tpl={tpl} used={usedByTpl(tpl.id)} disabled={busy === tpl.id} onCreate={() => create(tpl.id)} />
          ))}
          {templates && templates.length === 0 && (
            <Card><CardContent className="p-4 text-sm text-muted-foreground">No templates configured.</CardContent></Card>
          )}
        </div>
      </div>
      <Card>
        <CardHeader><CardTitle>{t("myDevPods")}</CardTitle></CardHeader>
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
                  <DevPodRow key={inst.name} inst={inst}
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
