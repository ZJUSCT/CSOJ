"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { DevPodTemplate } from "@/lib/types";
import useSWR from "swr";

const fetcher = (url: string) => api.get(url).then(r => r.data.data);
const empty: DevPodTemplate = { id:"",name:"",cluster_name:"",image:"ubuntu:24.04",shell:"bash",cores:4,memory:8<<30,node_selector:{},tolerations:[],default_per_user:1,default_global:5,persistence_size:"" };

export function CreateTemplateButton({ onDone }: { onDone: () => void }) {
  const [open, setOpen] = useState(false);
  const [v, setV] = useState<DevPodTemplate>(empty);
  const { toast } = useToast();
  const { data: clusters } = useSWR<{name:string}[]>("/admin/clusters", fetcher);
  const save = async () => {
    try { await api.post("/admin/devpod-templates", v); toast({title:"Created"}); setOpen(false); setV(empty); onDone(); }
    catch(e:any){ toast({variant:"destructive",title:"Failed",description:e.response?.data?.message}); }
  };
  const set = (k: keyof DevPodTemplate, val: any) => setV(s => ({...s, [k]: val}));
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button>New Template</Button></DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader><DialogTitle>New Template</DialogTitle></DialogHeader>
        <div className="grid grid-cols-2 gap-3">
          <L label="ID"><Input value={v.id} onChange={e=>set("id",e.target.value)} placeholder="gpu-8c"/></L>
          <L label="Name"><Input value={v.name} onChange={e=>set("name",e.target.value)}/></L>
          <L label="Cluster">
            <Select value={v.cluster_name} onValueChange={x=>set("cluster_name",x)}>
              <SelectTrigger><SelectValue placeholder="cluster"/></SelectTrigger>
              <SelectContent>{(clusters??[]).map(c=><SelectItem key={c.name} value={c.name}>{c.name}</SelectItem>)}</SelectContent>
            </Select>
          </L>
          <L label="Image"><Input value={v.image} onChange={e=>set("image",e.target.value)}/></L>
          <L label="Shell"><Input value={v.shell} onChange={e=>set("shell",e.target.value)} placeholder="bash|zsh|fish"/></L>
          <L label="Cores"><Input type="number" value={v.cores} onChange={e=>set("cores",+e.target.value)}/></L>
          <L label="Memory (bytes)"><Input type="number" value={v.memory} onChange={e=>set("memory",+e.target.value)}/></L>
          <L label="Per-user limit"><Input type="number" value={v.default_per_user} onChange={e=>set("default_per_user",+e.target.value)}/></L>
          <L label="Global limit"><Input type="number" value={v.default_global} onChange={e=>set("default_global",+e.target.value)}/></L>
          <L label="Persistence size"><Input value={v.persistence_size} onChange={e=>set("persistence_size",e.target.value)} placeholder="20Gi (blank=off)"/></L>
          <div className="col-span-2"><L label="Node Selector (JSON)"><Textarea className="font-mono text-xs" rows={2} value={JSON.stringify(v.node_selector)} onChange={e=>{try{set("node_selector",JSON.parse(e.target.value))}catch{}}} placeholder='{"numa-node":"0"}'/></L></div>
          <div className="col-span-2"><L label="Tolerations (JSON)"><Textarea className="font-mono text-xs" rows={3} value={JSON.stringify(v.tolerations)} onChange={e=>{try{set("tolerations",JSON.parse(e.target.value))}catch{}}}/></L></div>
        </div>
        <DialogFooter><Button onClick={save}>Save</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
function L({label,children}:{label:string;children:React.ReactNode}){return(<div><label className="text-xs">{label}</label>{children}</div>);}
