"use client";
import useSWR from "swr";
import api from "@/lib/api";
import { DevPodTemplate } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { CreateTemplateButton, EditTemplateButton } from "@/components/admin/devpod-template-actions";

const fetcher = (url: string) => api.get(url).then(r => r.data.data);

export default function DevPodTemplatesPage() {
  const { data: templates, mutate } = useSWR<DevPodTemplate[]>("/admin/devpod-templates", fetcher);
  const del = async (id: string) => { if (confirm("Delete template?")) { await api.delete(`/admin/devpod-templates/${id}`); mutate(); } };
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between"><CardTitle>DevPod Templates</CardTitle><CreateTemplateButton onDone={() => mutate()} /></CardHeader>
      <CardContent>
        <Table>
          <TableHeader><TableRow>
            <TableHead>ID</TableHead><TableHead>Name</TableHead><TableHead>Cluster</TableHead>
            <TableHead>Cores</TableHead><TableHead>Mem</TableHead><TableHead>GPU</TableHead><TableHead>Allowed Tags</TableHead><TableHead>PerUser</TableHead><TableHead>Global Running</TableHead><TableHead></TableHead>
          </TableRow></TableHeader>
          <TableBody>
            {(templates ?? []).map(t => (
              <TableRow key={t.id}>
                <TableCell className="font-mono text-xs">{t.id}</TableCell>
                <TableCell>{t.name}</TableCell>
                <TableCell>{t.cluster_name}</TableCell>
                <TableCell>{t.cores}</TableCell>
                <TableCell>{(t.memory/(1<<30)).toFixed(0)}Gi</TableCell>
                <TableCell className="whitespace-nowrap">{t.gpu_count > 0 ? `${t.gpu_count} × ${t.gpu_resource || "nvidia.com/gpu"}` : "-"}</TableCell>
                <TableCell>{t.allowed_tags?.length ? t.allowed_tags.join(", ") : "All users"}</TableCell>
                <TableCell>{t.default_per_user}</TableCell>
                <TableCell>{t.default_global}</TableCell>
                <TableCell>
                  <div className="flex items-center justify-end gap-1">
                    <EditTemplateButton template={t} onDone={() => mutate()} />
                    <Button size="sm" variant="ghost" className="text-destructive" onClick={() => del(t.id)}>Delete</Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
