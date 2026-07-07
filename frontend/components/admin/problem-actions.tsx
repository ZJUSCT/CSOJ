"use client";

import { useState, useEffect, useCallback } from "react";
import { useForm, useFieldArray } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Contest, Problem } from "@/lib/types";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { format } from "date-fns";
import { PlusCircle, Trash2, ChevronUp, ChevronDown, Code2, Server, HardDrive, Network, Zap } from "lucide-react";

// ---------- Types ----------

interface MountEntry {
    type: string;
    source: string;
    target: string;
    readonly: boolean;
}

interface MPIConfig {
    enabled: boolean;
    worker_replicas: number;
    slots_per_worker: number;
    launcher_cmd: string[];
}

interface WorkflowStepEntry {
    name: string;
    image: string;
    root: boolean;
    timeout: number;
    show: boolean;
    network: boolean;
    steps: string[][];   // array of commands; each command is string[]
    mounts: MountEntry[];
    mpi?: MPIConfig | null;
}

// ---------- Zod Schema ----------

const problemSchema = z.object({
    id: z.string().min(1, "ID is required").regex(/^[a-z0-9-_]+$/, "ID must be lowercase alphanumeric with hyphens"),
    name: z.string().min(1, "Name is required"),
    level: z.string().optional(),
    starttime: z.string().refine((val) => !isNaN(Date.parse(val)), "Invalid start time"),
    endtime: z.string().refine((val) => !isNaN(Date.parse(val)), "Invalid end time"),
    max_submissions: z.coerce.number().int().min(0, "Must be 0 or more"),
    cluster: z.string().min(1, "Cluster is required"),
    cpu: z.coerce.number().int().min(1, "CPU must be at least 1"),
    memory: z.coerce.number().int().min(1, "Memory must be at least 1"),
    description: z.string().optional(),
    score: z.object({
        mode: z.string().min(1, "Score mode is required"),
        max_performance_score: z.coerce.number().int().min(0),
    }),
    upload: z.object({
        max_num: z.coerce.number().int().min(0),
        max_size: z.coerce.number().int().min(0),
        upload_form: z.boolean(),
        upload_files: z.string(),
        editor: z.boolean(),
        editor_files: z.string(),
    }),
});

type ProblemFormValues = z.infer<typeof problemSchema>;

// ---------- Conversion helpers ----------

function workflowStepsToEntries(steps: any[]): WorkflowStepEntry[] {
    if (!steps || steps.length === 0) return [emptyStep()];
    return steps.map((s: any) => ({
        name: s.name || '',
        image: s.image || '',
        root: s.root ?? false,
        timeout: s.timeout ?? 30,
        show: s.show ?? true,
        network: s.network ?? false,
        steps: (s.steps || []).map((cmd: string[]) => cmd || []),
        mounts: (s.mounts || []).map((m: any) => ({
            type: m.type || 'bind',
            source: m.source || '',
            target: m.target || '',
            readonly: m.readonly ?? true,
        })),
        mpi: s.mpi ? {
            enabled: s.mpi.enabled ?? false,
            worker_replicas: s.mpi.worker_replicas ?? 1,
            slots_per_worker: s.mpi.slots_per_worker ?? 1,
            launcher_cmd: s.mpi.launcher_cmd || [],
        } : null,
    }));
}

function entriesToWorkflowSteps(entries: WorkflowStepEntry[]): any[] {
    return entries.map(e => {
        const step: any = {
            name: e.name,
            image: e.image || undefined,
            root: e.root,
            timeout: e.timeout,
            show: e.show,
            network: e.network,
            steps: e.steps,
        };
        if (e.mounts && e.mounts.length > 0) {
            step.mounts = e.mounts.map(m => ({
                type: m.type,
                source: m.source,
                target: m.target,
                readonly: m.readonly,
            }));
        }
        if (e.mpi && e.mpi.enabled) {
            step.mpi = e.mpi;
        }
        return step;
    });
}

function emptyStep(): WorkflowStepEntry {
    return {
        name: '', image: '', root: false, timeout: 30, show: true, network: false,
        steps: [], mounts: [], mpi: null,
    };
}

// ---------- Component ----------

export function ProblemFormDialog({
    problem,
    contestId,
    contests,
    onSuccess,
    trigger
}: {
    problem?: Problem,
    contestId?: string,
    contests: Contest[],
    onSuccess: () => void,
    trigger: React.ReactNode
}) {
    const [open, setOpen] = useState(false);
    const [selectedContest, setSelectedContest] = useState<string | undefined>(contestId);
    const [workflowSteps, setWorkflowSteps] = useState<WorkflowStepEntry[]>([emptyStep()]);
    const { toast } = useToast();
    const isEditing = !!problem;

    const form = useForm<ProblemFormValues>({
        resolver: zodResolver(problemSchema),
        defaultValues: {
            id: problem?.id || '',
            name: problem?.name || '',
            level: problem?.level || '',
            starttime: problem ? format(new Date(problem.starttime), "yyyy-MM-dd'T'HH:mm") : '',
            endtime: problem ? format(new Date(problem.endtime), "yyyy-MM-dd'T'HH:mm") : '',
            max_submissions: problem?.max_submissions || 0,
            cluster: problem?.cluster || '',
            cpu: problem?.cpu || 1,
            memory: problem?.memory || 128,
            description: problem?.description || '',
            score: {
                mode: problem?.score?.mode || 'score',
                max_performance_score: problem?.score?.max_performance_score || 100,
            },
            upload: {
                max_num: problem?.upload?.max_num || 1,
                max_size: problem?.upload?.max_size || 1024,
                upload_form: problem?.upload?.upload_form ?? true,
                upload_files: problem?.upload?.upload_files?.join(', ') || '',
                editor: problem?.upload?.editor ?? false,
                editor_files: problem?.upload?.editor_files?.join(', ') || '',
            },
        },
    });

    useEffect(() => {
        if (open) {
            form.reset({
                id: problem?.id || '',
                name: problem?.name || '',
                level: problem?.level || '',
                starttime: problem ? format(new Date(problem.starttime), "yyyy-MM-dd'T'HH:mm") : '',
                endtime: problem ? format(new Date(problem.endtime), "yyyy-MM-dd'T'HH:mm") : '',
                max_submissions: problem?.max_submissions || 0,
                cluster: problem?.cluster || '',
                cpu: problem?.cpu || 1,
                memory: problem?.memory || 128,
                description: problem?.description || '',
                score: {
                    mode: problem?.score?.mode || 'score',
                    max_performance_score: problem?.score?.max_performance_score || 100,
                },
                upload: {
                    max_num: problem?.upload?.max_num || 1,
                    max_size: problem?.upload?.max_size || 1024,
                    upload_form: problem?.upload?.upload_form ?? true,
                    upload_files: problem?.upload?.upload_files?.join(', ') || '',
                    editor: problem?.upload?.editor ?? false,
                    editor_files: problem?.upload?.editor_files?.join(', ') || '',
                },
            });
            setWorkflowSteps(problem?.workflow ? workflowStepsToEntries(problem.workflow) : [emptyStep()]);
        }
    }, [open, problem, form]);

    const onSubmit = async (values: ProblemFormValues) => {
        const finalContestId = isEditing ? contestId : selectedContest;
        if (!finalContestId) {
            toast({ variant: "destructive", title: "Error", description: "A parent contest must be selected." });
            return;
        }

        const payload = {
            ...values,
            starttime: new Date(values.starttime).toISOString(),
            endtime: new Date(values.endtime).toISOString(),
            workflow: entriesToWorkflowSteps(workflowSteps),
            upload: {
                ...values.upload,
                upload_files: values.upload.upload_files.split(',').map(s => s.trim()).filter(Boolean),
                editor_files: values.upload.editor_files.split(',').map(s => s.trim()).filter(Boolean),
            }
        };

        try {
            if (isEditing) {
                await api.put(`/admin/problems/${problem.id}`, payload);
            } else {
                await api.post(`/admin/contests/${finalContestId}/problems`, payload);
            }
            toast({ title: `Problem ${isEditing ? 'Updated' : 'Created'}`, description: `Problem "${payload.name}" has been saved.` });
            onSuccess();
            setOpen(false);
            form.reset();
        } catch (err: any) {
            toast({ variant: "destructive", title: "Operation Failed", description: err.response?.data?.message });
        }
    };

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>{trigger}</DialogTrigger>
            <DialogContent className="max-w-5xl max-h-[90vh] overflow-y-auto">
                <DialogHeader>
                    <DialogTitle>{isEditing ? "Edit Problem" : "Create New Problem"}</DialogTitle>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                        {!isEditing && (
                            <FormItem>
                                <FormLabel>Parent Contest</FormLabel>
                                <Select onValueChange={setSelectedContest} defaultValue={selectedContest}>
                                    <FormControl><SelectTrigger><SelectValue placeholder="Select a parent contest" /></SelectTrigger></FormControl>
                                    <SelectContent>
                                        {contests.map(c => <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>)}
                                    </SelectContent>
                                </Select>
                            </FormItem>
                        )}

                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <FormField control={form.control} name="id" render={({ field }) => (<FormItem><FormLabel>ID</FormLabel><FormControl><Input {...field} disabled={isEditing} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="name" render={({ field }) => (<FormItem><FormLabel>Name</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="starttime" render={({ field }) => (<FormItem><FormLabel>Start Time</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="endtime" render={({ field }) => (<FormItem><FormLabel>End Time</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="level" render={({ field }) => (<FormItem><FormLabel>Level</FormLabel><FormControl><Input placeholder="e.g., Easy, Medium, Hard" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="cluster" render={({ field }) => (<FormItem><FormLabel>Cluster</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="max_submissions" render={({ field }) => (<FormItem><FormLabel>Max Submissions</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="cpu" render={({ field }) => (<FormItem><FormLabel>CPU Cores</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="memory" render={({ field }) => (<FormItem><FormLabel>Memory (MB)</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                        </div>

                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 border p-4 rounded-md">
                            <h3 className="md:col-span-2 font-semibold">Score Settings</h3>
                            <FormField control={form.control} name="score.mode" render={({ field }) => (<FormItem><FormLabel>Score Mode</FormLabel><Select onValueChange={field.onChange} defaultValue={field.value}><FormControl><SelectTrigger><SelectValue /></SelectTrigger></FormControl><SelectContent><SelectItem value="score">score</SelectItem><SelectItem value="performance">performance</SelectItem></SelectContent></Select><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="score.max_performance_score" render={({ field }) => (<FormItem><FormLabel>Max Performance Score</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                        </div>

                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 border p-4 rounded-md">
                            <h3 className="md:col-span-2 font-semibold">Upload Settings</h3>
                            <FormField control={form.control} name="upload.max_num" render={({ field }) => (<FormItem><FormLabel>Max Files</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="upload.max_size" render={({ field }) => (<FormItem><FormLabel>Max Size (MB)</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="upload.upload_form" render={({ field }) => (<FormItem className="flex flex-row items-center space-x-2 space-y-0"><FormControl><Checkbox checked={field.value} onCheckedChange={field.onChange} /></FormControl><FormLabel className="!mt-0">Enable Upload Form</FormLabel></FormItem>)} />
                            <FormField control={form.control} name="upload.editor" render={({ field }) => (<FormItem className="flex flex-row items-center space-x-2 space-y-0"><FormControl><Checkbox checked={field.value} onCheckedChange={field.onChange} /></FormControl><FormLabel className="!mt-0">Enable Web Editor</FormLabel></FormItem>)} />
                            <FormField control={form.control} name="upload.upload_files" render={({ field }) => (<FormItem><FormLabel>Upload Files (comma-separated)</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="upload.editor_files" render={({ field }) => (<FormItem><FormLabel>Editable Files (comma-separated)</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
                        </div>

                        <FormField control={form.control} name="description" render={({ field }) => (
                            <FormItem><FormLabel>Description (Markdown)</FormLabel><FormControl><Textarea {...field} rows={8} /></FormControl><FormMessage /></FormItem>
                        )} />

                        {/* ===== Structured Workflow Editor ===== */}
                        <WorkflowEditor steps={workflowSteps} setSteps={setWorkflowSteps} />

                        <DialogFooter>
                            <Button type="submit" disabled={form.formState.isSubmitting}>{form.formState.isSubmitting ? "Saving..." : "Save"}</Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}

// ---------- Workflow Editor ----------

function WorkflowEditor({ steps, setSteps }: { steps: WorkflowStepEntry[], setSteps: (s: WorkflowStepEntry[]) => void }) {
    const addStep = () => setSteps([...steps, emptyStep()]);
    const removeStep = (i: number) => setSteps(steps.filter((_, idx) => idx !== i));
    const moveStep = (i: number, dir: -1 | 1) => {
        const j = i + dir;
        if (j < 0 || j >= steps.length) return;
        const arr = [...steps];
        [arr[i], arr[j]] = [arr[j], arr[i]];
        setSteps(arr);
    };
    const updateStep = (i: number, patch: Partial<WorkflowStepEntry>) => {
        const arr = [...steps];
        arr[i] = { ...arr[i], ...patch };
        setSteps(arr);
    };

    return (
        <div className="border p-4 rounded-md space-y-4">
            <div className="flex items-center justify-between">
                <h3 className="font-semibold flex items-center gap-2"><Code2 className="h-4 w-4" /> Workflow Steps</h3>
                <Button type="button" variant="outline" size="sm" onClick={addStep}><PlusCircle className="h-4 w-4 mr-1" /> Add Step</Button>
            </div>

            {steps.map((step, i) => (
                <StepCard
                    key={i}
                    index={i}
                    total={steps.length}
                    step={step}
                    onChange={(patch) => updateStep(i, patch)}
                    onRemove={() => removeStep(i)}
                    onMoveUp={() => moveStep(i, -1)}
                    onMoveDown={() => moveStep(i, 1)}
                />
            ))}

            {steps.length === 0 && (
                <p className="text-muted-foreground text-sm text-center py-4">No workflow steps. Click "Add Step" to create one.</p>
            )}
        </div>
    );
}

// ---------- Single Step Card ----------

function StepCard({ index, total, step, onChange, onRemove, onMoveUp, onMoveDown }: {
    index: number;
    total: number;
    step: WorkflowStepEntry;
    onChange: (patch: Partial<WorkflowStepEntry>) => void;
    onRemove: () => void;
    onMoveUp: () => void;
    onMoveDown: () => void;
}) {
    return (
        <div className="border rounded-md p-4 space-y-3 bg-muted/30">
            {/* Header */}
            <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                    <Badge variant="secondary">Step {index + 1}</Badge>
                    {step.mpi?.enabled && <Badge variant="default">MPI</Badge>}
                </div>
                <div className="flex items-center gap-1">
                    <Button type="button" variant="ghost" size="icon" className="h-7 w-7" disabled={index === 0} onClick={onMoveUp}><ChevronUp className="h-4 w-4" /></Button>
                    <Button type="button" variant="ghost" size="icon" className="h-7 w-7" disabled={index === total - 1} onClick={onMoveDown}><ChevronDown className="h-4 w-4" /></Button>
                    <Button type="button" variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={onRemove}><Trash2 className="h-4 w-4" /></Button>
                </div>
            </div>

            {/* Basic fields */}
            <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
                <div>
                    <Label>Step Name</Label>
                    <Input value={step.name} onChange={e => onChange({ name: e.target.value })} placeholder="compile" />
                </div>
                <div>
                    <Label>Image</Label>
                    <Input value={step.image} onChange={e => onChange({ image: e.target.value })} placeholder="gcc:13" />
                </div>
                <div>
                    <Label>Timeout (seconds)</Label>
                    <Input type="number" value={step.timeout} onChange={e => onChange({ timeout: parseInt(e.target.value) || 30 })} />
                </div>
            </div>

            {/* Toggles */}
            <div className="flex flex-wrap gap-4">
                <div className="flex items-center space-x-2">
                    <Checkbox id={`root-${index}`} checked={step.root} onCheckedChange={(v) => onChange({ root: !!v })} />
                    <Label htmlFor={`root-${index}`}>Run as root</Label>
                </div>
                <div className="flex items-center space-x-2">
                    <Checkbox id={`show-${index}`} checked={step.show} onCheckedChange={(v) => onChange({ show: !!v })} />
                    <Label htmlFor={`show-${index}`}>Show logs to user</Label>
                </div>
                <div className="flex items-center space-x-2">
                    <Checkbox id={`net-${index}`} checked={step.network} onCheckedChange={(v) => onChange({ network: !!v })} />
                    <Label htmlFor={`net-${index}`}>Enable network</Label>
                </div>
            </div>

            <Separator />

            {/* Commands */}
            <CommandsEditor
                commands={step.steps}
                onChange={(cmds) => onChange({ steps: cmds })}
            />

            <Separator />

            {/* Mounts */}
            <MountsEditor
                mounts={step.mounts}
                onChange={(m) => onChange({ mounts: m })}
            />

            <Separator />

            {/* MPI */}
            <MPIEditor
                mpi={step.mpi ?? null}
                onChange={(m) => onChange({ mpi: m })}
            />
        </div>
    );
}

// ---------- Commands Editor ----------

function CommandsEditor({ commands, onChange }: { commands: string[][], onChange: (cmds: string[][]) => void }) {
    const addCmd = () => onChange([...commands, []]);
    const removeCmd = (i: number) => onChange(commands.filter((_, idx) => idx !== i));
    const updateCmd = (i: number, text: string) => {
        const arr = [...commands];
        arr[i] = text.split(' ').filter(s => s.length > 0);
        onChange(arr);
    };

    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between">
                <Label className="flex items-center gap-1"><Zap className="h-3 w-3" /> Commands</Label>
                <Button type="button" variant="outline" size="sm" onClick={addCmd}><PlusCircle className="h-3 w-3 mr-1" /> Add Command</Button>
            </div>
            {commands.map((cmd, i) => (
                <div key={i} className="flex items-center gap-2">
                    <Badge variant="outline" className="text-xs">{i + 1}</Badge>
                    <Input
                        className="font-mono text-sm"
                        value={cmd.join(' ')}
                        onChange={e => updateCmd(i, e.target.value)}
                        placeholder="gcc main.c -o main"
                    />
                    <Button type="button" variant="ghost" size="icon" className="h-8 w-8 text-destructive shrink-0" onClick={() => removeCmd(i)}><Trash2 className="h-3 w-3" /></Button>
                </div>
            ))}
            {commands.length === 0 && <p className="text-xs text-muted-foreground">No commands. Add one to run in this step.</p>}
        </div>
    );
}

// ---------- Mounts Editor ----------

function MountsEditor({ mounts, onChange }: { mounts: MountEntry[], onChange: (m: MountEntry[]) => void }) {
    const addMount = () => onChange([...mounts, { type: 'bind', source: '', target: '', readonly: true }]);
    const removeMount = (i: number) => onChange(mounts.filter((_, idx) => idx !== i));
    const updateMount = (i: number, patch: Partial<MountEntry>) => {
        const arr = [...mounts];
        arr[i] = { ...arr[i], ...patch };
        onChange(arr);
    };

    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between">
                <Label className="flex items-center gap-1"><HardDrive className="h-3 w-3" /> Mounts</Label>
                <Button type="button" variant="outline" size="sm" onClick={addMount}><PlusCircle className="h-3 w-3 mr-1" /> Add Mount</Button>
            </div>
            {mounts.map((m, i) => (
                <div key={i} className="grid grid-cols-[80px_1fr_1fr_80px_32px] gap-2 items-center">
                    <Select value={m.type} onValueChange={(v) => updateMount(i, { type: v })}>
                        <SelectTrigger className="text-xs"><SelectValue /></SelectTrigger>
                        <SelectContent>
                            <SelectItem value="bind">bind</SelectItem>
                            <SelectItem value="volume">volume</SelectItem>
                            <SelectItem value="tmpfs">tmpfs</SelectItem>
                        </SelectContent>
                    </Select>
                    <Input className="text-xs" value={m.source} onChange={e => updateMount(i, { source: e.target.value })} placeholder="/host/path" />
                    <Input className="text-xs" value={m.target} onChange={e => updateMount(i, { target: e.target.value })} placeholder="/container/path" />
                    <div className="flex items-center space-x-1">
                        <Checkbox id={`ro-${i}`} checked={m.readonly} onCheckedChange={(v) => updateMount(i, { readonly: !!v })} />
                        <Label htmlFor={`ro-${i}`} className="text-xs">RO</Label>
                    </div>
                    <Button type="button" variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => removeMount(i)}><Trash2 className="h-3 w-3" /></Button>
                </div>
            ))}
            {mounts.length === 0 && <p className="text-xs text-muted-foreground">No mounts.</p>}
        </div>
    );
}

// ---------- MPI Editor ----------

function MPIEditor({ mpi, onChange }: { mpi: MPIConfig | null, onChange: (m: MPIConfig | null) => void }) {
    const enabled = mpi?.enabled ?? false;
    const toggle = () => {
        if (enabled) {
            onChange(null);
        } else {
            onChange({ enabled: true, worker_replicas: 1, slots_per_worker: 1, launcher_cmd: [] });
        }
    };
    const update = (patch: Partial<MPIConfig>) => {
        if (!mpi) return;
        onChange({ ...mpi, ...patch });
    };

    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between">
                <Label className="flex items-center gap-1"><Server className="h-3 w-3" /> MPI (multi-node)</Label>
                <div className="flex items-center space-x-2">
                    <Checkbox id="mpi-enabled" checked={enabled} onCheckedChange={(v) => toggle()} />
                    <Label htmlFor="mpi-enabled" className="text-xs">Enable MPI</Label>
                </div>
            </div>
            {enabled && mpi && (
                <div className="grid grid-cols-3 gap-3 pl-4 border-l-2 border-primary/20">
                    <div>
                        <Label>Worker Replicas</Label>
                        <Input type="number" value={mpi.worker_replicas} onChange={e => update({ worker_replicas: parseInt(e.target.value) || 1 })} />
                    </div>
                    <div>
                        <Label>Slots per Worker</Label>
                        <Input type="number" value={mpi.slots_per_worker} onChange={e => update({ slots_per_worker: parseInt(e.target.value) || 1 })} />
                    </div>
                    <div>
                        <Label>Launcher Command</Label>
                        <Input className="font-mono text-sm" value={mpi.launcher_cmd.join(' ')} onChange={e => update({ launcher_cmd: e.target.value.split(' ').filter(s => s.length > 0) })} placeholder="./a.out" />
                    </div>
                    <div className="col-span-3 text-xs text-muted-foreground">
                        Total MPI ranks: {mpi.worker_replicas * mpi.slots_per_worker}. The launcher runs <code className="font-mono">mpirun -np {mpi.worker_replicas * mpi.slots_per_worker} {mpi.launcher_cmd.join(' ')}</code>
                    </div>
                </div>
            )}
        </div>
    );
}

// ---------- Delete Button (unchanged) ----------

export function DeleteProblemButton({ problem, onSuccess, trigger }: { problem: Problem, onSuccess: () => void, trigger: React.ReactNode }) {
    const { toast } = useToast();
    const handleDelete = async () => {
        try {
            await api.delete(`/admin/problems/${problem.id}`);
            toast({ title: "Problem Deleted", description: `Problem "${problem.name}" has been deleted.` });
            onSuccess();
        } catch (err: any) {
            toast({ variant: "destructive", title: "Delete Failed", description: err.response?.data?.message });
        }
    }
    return (
        <AlertDialog>
            <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>
            <AlertDialogContent>
                <AlertDialogHeader><AlertDialogTitle>Are you absolutely sure?</AlertDialogTitle></AlertDialogHeader>
                <AlertDialogDescription>
                    This will permanently delete the problem `{problem.name}` and its files from the disk.
                </AlertDialogDescription>
                <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction onClick={handleDelete} className="bg-destructive hover:bg-destructive/90">Delete</AlertDialogAction>
                </AlertDialogFooter>
            </AlertDialogContent>
        </AlertDialog>
    );
}
