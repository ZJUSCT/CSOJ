"use client";

import { useState } from "react";
import useSWR from "swr";
import { Loader2, Pencil } from "lucide-react";

import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { DevPodTemplate } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

const fetcher = (url: string) => api.get(url).then(response => response.data.data);

const emptyTemplate: DevPodTemplate = {
  id: "",
  name: "",
  cluster_name: "",
  image: "ubuntu:24.04",
  shell: "bash",
  cores: 4,
  memory: 8 * 1024 * 1024 * 1024,
  gpu_count: 0,
  gpu_resource: "nvidia.com/gpu",
  node_selector: {},
  tolerations: [],
  allowed_tags: [],
  default_per_user: 1,
  default_global: 5,
};

function cloneTemplate(template?: DevPodTemplate): DevPodTemplate {
  const source = template ?? emptyTemplate;
  return {
    id: source.id,
    name: source.name,
    cluster_name: source.cluster_name,
    image: source.image,
    shell: source.shell,
    cores: source.cores,
    memory: source.memory,
    gpu_count: source.gpu_count ?? 0,
    gpu_resource: source.gpu_resource || "nvidia.com/gpu",
    node_selector: { ...(source.node_selector ?? {}) },
    tolerations: Array.isArray(source.tolerations) ? structuredClone(source.tolerations) : [],
    allowed_tags: [...(source.allowed_tags ?? [])],
    default_per_user: source.default_per_user,
    default_global: source.default_global,
  };
}

function TemplateDialog({ template, onDone }: { template?: DevPodTemplate; onDone: () => void }) {
  const editing = !!template;
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [value, setValue] = useState<DevPodTemplate>(() => cloneTemplate(template));
  const [nodeSelectorJSON, setNodeSelectorJSON] = useState(() => JSON.stringify(template?.node_selector ?? {}, null, 2));
  const [tolerationsJSON, setTolerationsJSON] = useState(() => JSON.stringify(template?.tolerations ?? [], null, 2));
  const [allowedTagsText, setAllowedTagsText] = useState(() => (template?.allowed_tags ?? []).join(", "));
  const { toast } = useToast();
  const { data: clusters } = useSWR<{ name: string }[]>("/admin/clusters", fetcher);

  const resetForm = () => {
    const next = cloneTemplate(template);
    setValue(next);
    setNodeSelectorJSON(JSON.stringify(next.node_selector, null, 2));
    setTolerationsJSON(JSON.stringify(next.tolerations, null, 2));
    setAllowedTagsText(next.allowed_tags.join(", "));
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) resetForm();
    setOpen(nextOpen);
  };

  const set = <K extends keyof DevPodTemplate>(key: K, next: DevPodTemplate[K]) => {
    setValue(current => ({ ...current, [key]: next }));
  };

  const save = async () => {
    let nodeSelector: unknown;
    let tolerations: unknown;
    try {
      nodeSelector = JSON.parse(nodeSelectorJSON);
    } catch {
      toast({ variant: "destructive", title: "Invalid Node Selector", description: "Node Selector must be valid JSON." });
      return;
    }
    try {
      tolerations = JSON.parse(tolerationsJSON);
    } catch {
      toast({ variant: "destructive", title: "Invalid Tolerations", description: "Tolerations must be valid JSON." });
      return;
    }
    if (!nodeSelector || typeof nodeSelector !== "object" || Array.isArray(nodeSelector) || Object.values(nodeSelector).some(item => typeof item !== "string")) {
      toast({ variant: "destructive", title: "Invalid Node Selector", description: "Use a JSON object whose values are strings." });
      return;
    }
    if (!Array.isArray(tolerations)) {
      toast({ variant: "destructive", title: "Invalid Tolerations", description: "Tolerations must be a JSON array." });
      return;
    }

    const payload: DevPodTemplate = {
      ...value,
      node_selector: nodeSelector as Record<string, string>,
      tolerations,
      allowed_tags: allowedTagsText.split(",").map(tag => tag.trim()).filter(Boolean),
    };
    setSaving(true);
    try {
      if (editing) {
        await api.put(`/admin/devpod-templates/${encodeURIComponent(value.id)}`, payload);
      } else {
        await api.post("/admin/devpod-templates", payload);
      }
      toast({ title: editing ? "Template updated" : "Template created" });
      setOpen(false);
      onDone();
    } catch (error: any) {
      toast({
        variant: "destructive",
        title: editing ? "Update failed" : "Create failed",
        description: error.response?.data?.message ?? error.message,
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        {editing ? (
          <Button size="sm" variant="outline"><Pencil className="mr-1.5 h-3.5 w-3.5" />Edit</Button>
        ) : (
          <Button>New Template</Button>
        )}
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader><DialogTitle>{editing ? `Edit Template: ${template.name}` : "New Template"}</DialogTitle></DialogHeader>
        <div className="grid grid-cols-2 gap-3">
          <Field label="ID"><Input value={value.id} disabled={editing} onChange={event => set("id", event.target.value)} placeholder="gpu-8c" /></Field>
          <Field label="Name"><Input value={value.name} onChange={event => set("name", event.target.value)} /></Field>
          <Field label="Cluster">
            <Select value={value.cluster_name} onValueChange={cluster => set("cluster_name", cluster)}>
              <SelectTrigger><SelectValue placeholder="cluster" /></SelectTrigger>
              <SelectContent>{(clusters ?? []).map(cluster => <SelectItem key={cluster.name} value={cluster.name}>{cluster.name}</SelectItem>)}</SelectContent>
            </Select>
          </Field>
          <Field label="Image"><Input value={value.image} onChange={event => set("image", event.target.value)} /></Field>
          <Field label="Shell"><Input value={value.shell} onChange={event => set("shell", event.target.value)} placeholder="bash|zsh|fish" /></Field>
          <Field label="Cores"><Input type="number" min={1} value={value.cores} onChange={event => set("cores", Number(event.target.value))} /></Field>
          <Field label="Memory (bytes)"><Input type="number" min={1} value={value.memory} onChange={event => set("memory", Number(event.target.value))} /></Field>
          <Field label="GPU count"><Input type="number" min={0} step={1} value={value.gpu_count} onChange={event => set("gpu_count", Number(event.target.value))} /></Field>
          <Field label="GPU resource"><Input className="font-mono" value={value.gpu_resource} onChange={event => set("gpu_resource", event.target.value)} placeholder="nvidia.com/gpu" /></Field>
          <Field label="Per-user limit"><Input type="number" min={1} value={value.default_per_user} onChange={event => set("default_per_user", Number(event.target.value))} /></Field>
          <Field label="Global running limit"><Input type="number" min={1} value={value.default_global} onChange={event => set("default_global", Number(event.target.value))} /></Field>
          <div className="col-span-2">
            <Field label="Allowed user tags">
              <Input
                value={allowedTagsText}
                onChange={event => setAllowedTagsText(event.target.value)}
                placeholder="gpu, hpc (leave empty for all users)"
              />
            </Field>
          </div>
          <div className="col-span-2"><Field label="Node Selector (JSON)"><Textarea className="font-mono text-xs" rows={3} value={nodeSelectorJSON} onChange={event => setNodeSelectorJSON(event.target.value)} placeholder='{"numa-node":"0"}' /></Field></div>
          <div className="col-span-2"><Field label="Tolerations (JSON)"><Textarea className="font-mono text-xs" rows={4} value={tolerationsJSON} onChange={event => setTolerationsJSON(event.target.value)} /></Field></div>
        </div>
        <DialogFooter>
          <Button onClick={save} disabled={saving}>
            {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {saving ? "Saving…" : editing ? "Save Changes" : "Create Template"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function CreateTemplateButton({ onDone }: { onDone: () => void }) {
  return <TemplateDialog onDone={onDone} />;
}

export function EditTemplateButton({ template, onDone }: { template: DevPodTemplate; onDone: () => void }) {
  return <TemplateDialog template={template} onDone={onDone} />;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="space-y-1"><label className="text-xs font-medium">{label}</label>{children}</div>;
}
