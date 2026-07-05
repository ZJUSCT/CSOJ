"use client";

import { useState, useEffect } from "react";
import { useForm, useFieldArray } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Contest, Problem, WorkflowStep } from "@/lib/types";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "../ui/textarea";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "../ui/alert-dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";
import yaml from 'js-yaml';
import { Label } from "../ui/label";
import { Checkbox } from "../ui/checkbox";
import { format } from "date-fns";

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
    workflow: z.string().refine((val) => {
        try {
            const parsed = yaml.load(val);
            return Array.isArray(parsed);
        } catch {
            return false;
        }
    }, "Must be a valid YAML array of workflow steps"),
});

type ProblemFormValues = z.infer<typeof problemSchema>;

export function ProblemFormDialog({
    problem,
    contestId, // required for creating a new problem
    contests, // list of all contests to choose from
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
            workflow: problem?.workflow ? yaml.dump(problem.workflow) : '- name: compile\n  image: gcc\n  steps:\n    - [ "gcc", "main.c", "-o", "main" ]\n',
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
                workflow: problem?.workflow ? yaml.dump(problem.workflow) : '- name: compile\n  image: gcc\n  steps:\n    - [ "gcc", "main.c", "-o", "main" ]\n',
            });
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
            workflow: yaml.load(values.workflow) as WorkflowStep[],
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
            <DialogContent className="max-w-4xl max-h-[90vh] overflow-y-auto">
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
                             <FormField control={form.control} name="level" render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Level</FormLabel>
                                    <FormControl><Input placeholder="e.g., Easy, Medium, Hard" {...field} /></FormControl>
                                    <FormMessage />
                                </FormItem>
                            )} />
                            <FormField control={form.control} name="cluster" render={({ field }) => (<FormItem><FormLabel>Cluster</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="max_submissions" render={({ field }) => (<FormItem><FormLabel>Max Submissions</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                             <FormField control={form.control} name="cpu" render={({ field }) => (<FormItem><FormLabel>CPU Cores</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                            <FormField control={form.control} name="memory" render={({ field }) => (<FormItem><FormLabel>Memory (MB)</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                        </div>
                        
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 border p-4 rounded-md">
                             <h3 className="md:col-span-2 font-semibold">Score Settings</h3>
                             <FormField control={form.control} name="score.mode" render={({ field }) => (<FormItem><FormLabel>Score Mode</FormLabel><Select onValueChange={field.onChange} defaultValue={field.value}><FormControl><SelectTrigger><SelectValue/></SelectTrigger></FormControl><SelectContent><SelectItem value="score">score</SelectItem><SelectItem value="performance">performance</SelectItem></SelectContent></Select><FormMessage /></FormItem>)} />
                             <FormField control={form.control} name="score.max_performance_score" render={({ field }) => (<FormItem><FormLabel>Max Performance Score</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                        </div>

                         <div className="grid grid-cols-1 md:grid-cols-2 gap-4 border p-4 rounded-md">
                             <h3 className="md:col-span-2 font-semibold">Upload Settings</h3>
                              <FormField control={form.control} name="upload.max_num" render={({ field }) => (<FormItem><FormLabel>Max Files</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                              <FormField control={form.control} name="upload.max_size" render={({ field }) => (<FormItem><FormLabel>Max Size (MB)</FormLabel><FormControl><Input type="number" {...field} /></FormControl><FormMessage /></FormItem>)} />
                              <FormField control={form.control} name="upload.upload_form" render={({ field }) => (
                                  <FormItem className="flex flex-row items-center space-x-2 space-y-0"><FormControl><Checkbox checked={field.value} onCheckedChange={field.onChange} /></FormControl><FormLabel className="!mt-0">Enable Upload Form</FormLabel></FormItem>
                              )} />
                              <FormField control={form.control} name="upload.editor" render={({ field }) => (
                                  <FormItem className="flex flex-row items-center space-x-2 space-y-0"><FormControl><Checkbox checked={field.value} onCheckedChange={field.onChange} /></FormControl><FormLabel className="!mt-0">Enable Web Editor</FormLabel></FormItem>
                              )} />
                              <FormField control={form.control} name="upload.upload_files" render={({ field }) => (<FormItem><FormLabel>Upload Files (comma-separated)</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
                              <FormField control={form.control} name="upload.editor_files" render={({ field }) => (<FormItem><FormLabel>Editable Files (comma-separated)</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
                        </div>

                        <FormField control={form.control} name="description" render={({ field }) => (
                            <FormItem><FormLabel>Description (Markdown)</FormLabel><FormControl><Textarea {...field} rows={8} /></FormControl><FormMessage /></FormItem>
                        )} />
                        <FormField control={form.control} name="workflow" render={({ field }) => (
                            <FormItem><FormLabel>Workflow (YAML)</FormLabel><FormControl><Textarea className="font-mono" {...field} rows={15} /></FormControl><FormMessage /></FormItem>
                        )} />

                        <DialogFooter>
                            <Button type="submit" disabled={form.formState.isSubmitting}>{form.formState.isSubmitting ? "Saving..." : "Save"}</Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}

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