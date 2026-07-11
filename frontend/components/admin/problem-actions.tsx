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
import { PlusCircle, Trash2, ChevronUp, ChevronDown, Code2, Server, HardDrive, Network, Zap, Cpu, Calendar } from "lucide-react";

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

interface DeadlineOverrideEntry {
    tags: string;
    end_time: string;
}

interface StepResourceState {
    cpu_request: string;
    cpu_limit: string;
    memory_request: string;
    memory_limit: string;
    gpu_count: number;
    gpu_resource: string;
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
    resources: StepResourceState;
    scheduling: {
        node_selector: { key: string; value: string }[];
        node_affinity: { key: string; operator: string; values: string }[];
        tolerations: { key: string; operator: string; value: string; effect: string }[];
        priority_class_name: string;
        runtime_class_name: string;
    };
}

// ---------- Zod Schema ----------

const problemSchema = z.object({
    id: z.string().min(1, "ID is required").regex(/^[a-z0-9-_]+$/, "ID must be lowercase alphanumeric with hyphens"),
    name: z.string().min(1, "Name is required"),
    level: z.string().optional(),
    starttime: z.string().refine((val) => !isNaN(Date.parse(val)), "Invalid start time"),
    endtime: z.string().refine((val) => !isNaN(Date.parse(val)), "Invalid end time"),
    submit_start_time: z.string().optional(),
    submit_end_time: z.string().optional(),
    max_submissions: z.coerce.number().int().min(0, "Must be 0 or more"),
    cluster: z.string().min(1, "Cluster is required"),
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
        resources: {
            cpu_request: s.resources?.cpu_request || '',
            cpu_limit: s.resources?.cpu_limit || '',
            memory_request: s.resources?.memory_request || '',
            memory_limit: s.resources?.memory_limit || '',
            gpu_count: s.resources?.gpu_count ?? 0,
            gpu_resource: s.resources?.gpu_resource || 'nvidia.com/gpu',
        },
        scheduling: {
            node_selector: Object.entries(s.scheduling?.node_selector || {}).map(([k, v]) => ({ key: k, value: v as string })),
            node_affinity: (s.scheduling?.node_affinity || []).map((t: any) => ({
                key: t.key || '',
                operator: t.operator || 'In',
                values: (t.values || []).join(', '),
            })),
            tolerations: (s.scheduling?.tolerations || []).map((t: any) => ({
                key: t.key || '',
                operator: t.operator || 'Equal',
                value: t.value || '',
                effect: t.effect || '(none)',
            })),
            priority_class_name: s.scheduling?.priority_class_name || '',
            runtime_class_name: s.scheduling?.runtime_class_name || '',
        },
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
        const res = e.resources;
        if (res.cpu_request || res.cpu_limit || res.memory_request || res.memory_limit || res.gpu_count > 0) {
            step.resources = {
                cpu_request: res.cpu_request || undefined,
                cpu_limit: res.cpu_limit || undefined,
                memory_request: res.memory_request || undefined,
                memory_limit: res.memory_limit || undefined,
                gpu_count: res.gpu_count > 0 ? res.gpu_count : undefined,
                gpu_resource: res.gpu_count > 0 ? (res.gpu_resource || 'nvidia.com/gpu') : undefined,
            };
        }
        const sched = e.scheduling;
        const nodeSelector = sched.node_selector.filter(p => p.key && p.value).reduce((acc, p) => { acc[p.key] = p.value; return acc; }, {} as Record<string, string>);
        const nodeAffinity = sched.node_affinity.filter(t => t.key).map(t => ({
            key: t.key,
            operator: t.operator,
            values: t.values.split(',').map(v => v.trim()).filter(Boolean),
        }));
        const tolerations = sched.tolerations.filter(t => t.key).map(t => ({
            key: t.key,
            operator: t.operator,
            value: t.value || undefined,
            effect: t.effect && t.effect !== '(none)' ? t.effect : undefined,
        }));
        if (Object.keys(nodeSelector).length > 0 || nodeAffinity.length > 0 || tolerations.length > 0 || sched.priority_class_name || sched.runtime_class_name) {
            step.scheduling = {
                ...(Object.keys(nodeSelector).length > 0 ? { node_selector: nodeSelector } : {}),
                ...(nodeAffinity.length > 0 ? { node_affinity: nodeAffinity } : {}),
                ...(tolerations.length > 0 ? { tolerations } : {}),
                ...(sched.priority_class_name ? { priority_class_name: sched.priority_class_name } : {}),
                ...(sched.runtime_class_name ? { runtime_class_name: sched.runtime_class_name } : {}),
            };
        }
        return step;
    });
}

function emptyStep(): WorkflowStepEntry {
    return {
        name: '', image: '', root: false, timeout: 30, show: true, network: false,
        steps: [], mounts: [], mpi: null,
        resources: { cpu_request: '', cpu_limit: '', memory_request: '', memory_limit: '', gpu_count: 0, gpu_resource: 'nvidia.com/gpu' },
        scheduling: { node_selector: [], node_affinity: [], tolerations: [], priority_class_name: '', runtime_class_name: '' },
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
    const [deadlineOverrides, setDeadlineOverrides] = useState<DeadlineOverrideEntry[]>(
        problem?.deadline_overrides?.map(o => ({ tags: o.tags.join(', '), end_time: o.end_time ? format(new Date(o.end_time), "yyyy-MM-dd'T'HH:mm") : '' })) || []
    );
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
            submit_start_time: problem?.submit_start_time ? format(new Date(problem.submit_start_time), "yyyy-MM-dd'T'HH:mm") : '',
            submit_end_time: problem?.submit_end_time ? format(new Date(problem.submit_end_time), "yyyy-MM-dd'T'HH:mm") : '',
            max_submissions: problem?.max_submissions || 0,
            cluster: problem?.cluster || '',
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
                submit_start_time: problem?.submit_start_time ? format(new Date(problem.submit_start_time), "yyyy-MM-dd'T'HH:mm") : '',
                submit_end_time: problem?.submit_end_time ? format(new Date(problem.submit_end_time), "yyyy-MM-dd'T'HH:mm") : '',
                max_submissions: problem?.max_submissions || 0,
                cluster: problem?.cluster || '',
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
            setDeadlineOverrides(problem?.deadline_overrides?.map(o => ({ tags: o.tags.join(', '), end_time: o.end_time ? format(new Date(o.end_time), "yyyy-MM-dd'T'HH:mm") : '' })) || []);
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
            submit_start_time: values.submit_start_time ? new Date(values.submit_start_time).toISOString() : null,
            submit_end_time: values.submit_end_time ? new Date(values.submit_end_time).toISOString() : null,
            workflow: entriesToWorkflowSteps(workflowSteps),
            deadline_overrides: deadlineOverrides.filter(o => o.tags.trim() !== '' && o.end_time !== '').map(o => ({
                tags: o.tags.split(',').map(t => t.trim()).filter(Boolean),
                end_time: new Date(o.end_time).toISOString(),
            })),
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
                            <FormField control={form.control} name="level" render={({ field }) => (<FormItem><FormLabel>Level</FormLabel><FormControl><Input placeholder="e.g., Easy, Medium, Hard" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="cluster" render={({ field }) => (<FormItem><FormLabel>Cluster</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="max_submissions" render={({ field }) => (<FormItem><FormLabel>Max Submissions</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                        </div>

                        <div className="border p-4 rounded-md space-y-4">
                            <h3 className="font-semibold">Time Windows</h3>
                            <div className="grid grid-cols-2 gap-4">
                                <FormField control={form.control} name="starttime" render={({ field }) => (
                                    <FormItem><FormLabel>Visible Start</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>
                                )} />
                                <FormField control={form.control} name="endtime" render={({ field }) => (
                                    <FormItem><FormLabel>Visible End</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>
                                )} />
                            </div>
                            <p className="text-xs text-muted-foreground">Submission window (optional — leave empty to use the same as visibility):</p>
                            <div className="grid grid-cols-2 gap-4">
                                <FormField control={form.control} name="submit_start_time" render={({ field }) => (
                                    <FormItem><FormLabel>Submit Start (optional)</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>
                                )} />
                                <FormField control={form.control} name="submit_end_time" render={({ field }) => (
                                    <FormItem><FormLabel>Submit End (optional)</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>
                                )} />
                            </div>
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

                        <DeadlineOverridesEditor overrides={deadlineOverrides} setOverrides={setDeadlineOverrides} />

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

            {/* Resources */}
            <ResourcesEditor
                res={step.resources}
                onChange={(r) => onChange({ resources: r })}
            />

            <Separator />

            {/* Scheduling */}
            <SchedulingEditor
                sched={step.scheduling}
                onChange={(s) => onChange({ scheduling: s })}
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

// ---------- Resources Editor ----------

function ResourcesEditor({ res, onChange }: {
    res: StepResourceState;
    onChange: (r: StepResourceState) => void;
}) {
    const update = (patch: Partial<typeof res>) => onChange({ ...res, ...patch });
    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between">
                <Label className="flex items-center gap-1"><Cpu className="h-3 w-3" /> Resources</Label>
                <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => update({ cpu_limit: res.cpu_request, memory_limit: res.memory_request })}
                >
                    Make Guaranteed
                </Button>
            </div>
            <div className="grid grid-cols-2 gap-2 pl-4">
                <div>
                    <Label className="text-xs">CPU Request</Label>
                    <Input className="text-xs font-mono" value={res.cpu_request} onChange={e => update({ cpu_request: e.target.value })} placeholder="2 / 500m" />
                </div>
                <div>
                    <Label className="text-xs">CPU Limit</Label>
                    <Input className="text-xs font-mono" value={res.cpu_limit} onChange={e => update({ cpu_limit: e.target.value })} placeholder="2" />
                </div>
                <div>
                    <Label className="text-xs">Memory Request</Label>
                    <Input className="text-xs font-mono" value={res.memory_request} onChange={e => update({ memory_request: e.target.value })} placeholder="1Gi / 256Mi" />
                </div>
                <div>
                    <Label className="text-xs">Memory Limit</Label>
                    <Input className="text-xs font-mono" value={res.memory_limit} onChange={e => update({ memory_limit: e.target.value })} placeholder="1Gi" />
                </div>
                <div>
                    <Label className="text-xs">GPU Count</Label>
                    <Input type="number" min={0} step={1} className="text-xs font-mono" value={res.gpu_count} onChange={e => update({ gpu_count: Number(e.target.value) })} />
                </div>
                <div>
                    <Label className="text-xs">GPU Resource</Label>
                    <Input className="text-xs font-mono" value={res.gpu_resource} onChange={e => update({ gpu_resource: e.target.value })} placeholder="nvidia.com/gpu" />
                </div>
            </div>
            <p className="text-xs text-muted-foreground pl-4">GPU is requested as an extended resource with request == limit. For MPI steps, the count applies to each launcher and worker Pod.</p>
        </div>
    );
}

// ---------- Scheduling Editor ----------

interface NodeSelectorPair { key: string; value: string }
interface NodeAffinityEntry { key: string; operator: string; values: string }
interface TolerationEntry { key: string; operator: string; value: string; effect: string }
interface SchedulingState {
    node_selector: NodeSelectorPair[];
    node_affinity: NodeAffinityEntry[];
    tolerations: TolerationEntry[];
    priority_class_name: string;
    runtime_class_name: string;
}

function SchedulingEditor({ sched, onChange }: { sched: SchedulingState, onChange: (s: SchedulingState) => void }) {
    const update = (patch: Partial<SchedulingState>) => onChange({ ...sched, ...patch });

    // --- Node Selector ---
    const addNodeSelector = () => update({ node_selector: [...sched.node_selector, { key: '', value: '' }] });
    const removeNodeSelector = (i: number) => update({ node_selector: sched.node_selector.filter((_, idx) => idx !== i) });
    const updateNodeSelector = (i: number, patch: Partial<NodeSelectorPair>) => {
        const arr = [...sched.node_selector];
        arr[i] = { ...arr[i], ...patch };
        update({ node_selector: arr });
    };

    // --- Node Affinity ---
    const addAffinity = () => update({ node_affinity: [...sched.node_affinity, { key: '', operator: 'In', values: '' }] });
    const removeAffinity = (i: number) => update({ node_affinity: sched.node_affinity.filter((_, idx) => idx !== i) });
    const updateAffinity = (i: number, patch: Partial<NodeAffinityEntry>) => {
        const arr = [...sched.node_affinity];
        arr[i] = { ...arr[i], ...patch };
        update({ node_affinity: arr });
    };

    // --- Tolerations ---
    const addToleration = () => update({ tolerations: [...sched.tolerations, { key: '', operator: 'Equal', value: '', effect: '' }] });
    const removeToleration = (i: number) => update({ tolerations: sched.tolerations.filter((_, idx) => idx !== i) });
    const updateToleration = (i: number, patch: Partial<TolerationEntry>) => {
        const arr = [...sched.tolerations];
        arr[i] = { ...arr[i], ...patch };
        update({ tolerations: arr });
    };

    return (
        <div className="space-y-3">
            <Label className="flex items-center gap-1"><Calendar className="h-3 w-3" /> Scheduling Constraints</Label>

            {/* Node Selector */}
            <div className="space-y-2 pl-4 border-l-2 border-primary/20">
                <div className="flex items-center justify-between">
                    <Label className="text-xs">Node Selector</Label>
                    <Button type="button" variant="outline" size="sm" onClick={addNodeSelector}><PlusCircle className="h-3 w-3 mr-1" /> Add</Button>
                </div>
                {sched.node_selector.map((pair, i) => (
                    <div key={i} className="grid grid-cols-[1fr_1fr_32px] gap-2 items-center">
                        <Input className="text-xs font-mono" value={pair.key} onChange={e => updateNodeSelector(i, { key: e.target.value })} placeholder="disktype" />
                        <Input className="text-xs font-mono" value={pair.value} onChange={e => updateNodeSelector(i, { value: e.target.value })} placeholder="ssd" />
                        <Button type="button" variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => removeNodeSelector(i)}><Trash2 className="h-3 w-3" /></Button>
                    </div>
                ))}
                {sched.node_selector.length === 0 && <p className="text-xs text-muted-foreground">No node selector.</p>}
            </div>

            {/* Node Affinity */}
            <div className="space-y-2 pl-4 border-l-2 border-primary/20">
                <div className="flex items-center justify-between">
                    <Label className="text-xs">Node Affinity (required, AND across terms)</Label>
                    <Button type="button" variant="outline" size="sm" onClick={addAffinity}><PlusCircle className="h-3 w-3 mr-1" /> Add</Button>
                </div>
                {sched.node_affinity.map((term, i) => (
                    <div key={i} className="grid grid-cols-[1fr_120px_1fr_32px] gap-2 items-center">
                        <Input className="text-xs font-mono" value={term.key} onChange={e => updateAffinity(i, { key: e.target.value })} placeholder="cpu-manager" />
                        <Select value={term.operator} onValueChange={(v) => updateAffinity(i, { operator: v })}>
                            <SelectTrigger className="text-xs"><SelectValue /></SelectTrigger>
                            <SelectContent>
                                <SelectItem value="In">In</SelectItem>
                                <SelectItem value="NotIn">NotIn</SelectItem>
                                <SelectItem value="Exists">Exists</SelectItem>
                                <SelectItem value="DoesNotExist">DoesNotExist</SelectItem>
                            </SelectContent>
                        </Select>
                        <Input className="text-xs font-mono" value={term.values} onChange={e => updateAffinity(i, { values: e.target.value })} placeholder="static (comma-separated)" />
                        <Button type="button" variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => removeAffinity(i)}><Trash2 className="h-3 w-3" /></Button>
                    </div>
                ))}
                {sched.node_affinity.length === 0 && <p className="text-xs text-muted-foreground">No node affinity.</p>}
            </div>

            {/* Tolerations */}
            <div className="space-y-2 pl-4 border-l-2 border-primary/20">
                <div className="flex items-center justify-between">
                    <Label className="text-xs">Tolerations</Label>
                    <Button type="button" variant="outline" size="sm" onClick={addToleration}><PlusCircle className="h-3 w-3 mr-1" /> Add</Button>
                </div>
                {sched.tolerations.map((t, i) => (
                    <div key={i} className="grid grid-cols-[1fr_100px_1fr_120px_32px] gap-2 items-center">
                        <Input className="text-xs font-mono" value={t.key} onChange={e => updateToleration(i, { key: e.target.value })} placeholder="dedicated" />
                        <Select value={t.operator} onValueChange={(v) => updateToleration(i, { operator: v })}>
                            <SelectTrigger className="text-xs"><SelectValue /></SelectTrigger>
                            <SelectContent>
                                <SelectItem value="Equal">Equal</SelectItem>
                                <SelectItem value="Exists">Exists</SelectItem>
                            </SelectContent>
                        </Select>
                        <Input className="text-xs font-mono" value={t.value} onChange={e => updateToleration(i, { value: e.target.value })} placeholder="cpu-pinning" />
                        <Select value={t.effect} onValueChange={(v) => updateToleration(i, { effect: v })}>
                            <SelectTrigger className="text-xs"><SelectValue placeholder="(none)" /></SelectTrigger>
                            <SelectContent>
                                <SelectItem value="(none)">(none)</SelectItem>
                                <SelectItem value="NoSchedule">NoSchedule</SelectItem>
                                <SelectItem value="NoExecute">NoExecute</SelectItem>
                                <SelectItem value="PreferNoSchedule">PreferNoSchedule</SelectItem>
                            </SelectContent>
                        </Select>
                        <Button type="button" variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => removeToleration(i)}><Trash2 className="h-3 w-3" /></Button>
                    </div>
                ))}
                {sched.tolerations.length === 0 && <p className="text-xs text-muted-foreground">No tolerations.</p>}
            </div>

            {/* Priority / Runtime Class */}
            <div className="grid grid-cols-2 gap-2 pl-4 border-l-2 border-primary/20">
                <div>
                    <Label className="text-xs">Priority Class Name</Label>
                    <Input className="text-xs font-mono" value={sched.priority_class_name} onChange={e => update({ priority_class_name: e.target.value })} placeholder="latency-critical" />
                </div>
                <div>
                    <Label className="text-xs">Runtime Class Name</Label>
                    <Input className="text-xs font-mono" value={sched.runtime_class_name} onChange={e => update({ runtime_class_name: e.target.value })} placeholder="runc" />
                </div>
            </div>
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

// ---------- Deadline Overrides Editor ----------

function DeadlineOverridesEditor({ overrides, setOverrides }: { overrides: DeadlineOverrideEntry[], setOverrides: (o: DeadlineOverrideEntry[]) => void }) {
    const addOverride = () => setOverrides([...overrides, { tags: '', end_time: '' }]);
    const removeOverride = (i: number) => setOverrides(overrides.filter((_, idx) => idx !== i));
    const updateOverride = (i: number, field: 'tags' | 'end_time', value: string) => {
        const next = [...overrides];
        next[i] = { ...next[i], [field]: value };
        setOverrides(next);
    };

    return (
        <div className="border p-4 rounded-md space-y-3">
            <div className="flex items-center justify-between">
                <h3 className="font-semibold">Deadline Overrides (optional)</h3>
                <Button type="button" variant="outline" size="sm" onClick={addOverride}><PlusCircle className="h-3 w-3 mr-1" /> Add Override</Button>
            </div>
            <p className="text-xs text-muted-foreground">Users with matching tags get a later submission deadline. Multiple matches take the latest.</p>
            {overrides.map((override, i) => (
                <div key={i} className="grid grid-cols-[1fr_1fr_32px] gap-2 items-center">
                    <Input className="text-xs" value={override.tags} onChange={e => updateOverride(i, 'tags', e.target.value)} placeholder="VIP, Staff" />
                    <Input className="text-xs" type="datetime-local" value={override.end_time} onChange={e => updateOverride(i, 'end_time', e.target.value)} />
                    <Button type="button" variant="ghost" size="icon" className="h-8 w-8 text-destructive" onClick={() => removeOverride(i)}><Trash2 className="h-3 w-3" /></Button>
                </div>
            ))}
            {overrides.length === 0 && <p className="text-xs text-muted-foreground">No overrides. Default submission window applies to all users.</p>}
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
