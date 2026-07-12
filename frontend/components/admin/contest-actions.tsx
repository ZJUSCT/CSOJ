"use client";

import { useState, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useSWRConfig } from "swr";
import { Contest, RegistrationConfig } from "@/lib/types";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "../ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { format } from "date-fns";

type RegistrationMode = "auto" | "tag_auto" | "tag_review" | "review";

const contestSchema = z.object({
    id: z.string().min(1, "ID is required").regex(/^[a-z0-9-_]+$/, "ID must be lowercase alphanumeric with hyphens"),
    name: z.string().min(1, "Name is required"),
    starttime: z.string().refine((val) => !isNaN(Date.parse(val)), "Invalid start time"),
    endtime: z.string().refine((val) => !isNaN(Date.parse(val)), "Invalid end time"),
    submit_start_time: z.string().optional(),
    submit_end_time: z.string().optional(),
    description: z.string().optional(),
    registration_mode: z.enum(["auto", "tag_auto", "tag_review", "review"]),
    registration_allowed_tags: z.string().optional(),
});

type ContestFormValues = z.infer<typeof contestSchema>;

const REGISTRATION_MODE_LABELS: Record<RegistrationMode, string> = {
    auto: "Auto (any user, instantly approved)",
    tag_auto: "Tag Auto (only tagged users, instantly approved)",
    tag_review: "Tag Review (tagged users auto-approved, others require review)",
    review: "Review (all registrations require admin review)",
};

function parseAllowedTags(value: string | undefined): string[] {
    if (!value) return [];
    return value
        .split(",")
        .map((t) => t.trim())
        .filter((t) => t.length > 0);
}

export function ContestFormDialog({
    contest,
    onSuccess,
    trigger
}: {
    contest?: Contest,
    onSuccess: () => void,
    trigger: React.ReactNode
}) {
    const [open, setOpen] = useState(false);
    const { toast } = useToast();
    const isEditing = !!contest;

    const existingMode: RegistrationMode = (contest?.registration_config?.mode as RegistrationMode) || "auto";
    const existingTags = contest?.registration_config?.allowed_tags?.join(", ") || "";

    const form = useForm<ContestFormValues>({
        resolver: zodResolver(contestSchema),
        defaultValues: {
            id: contest?.id || '',
            name: contest?.name || '',
            starttime: contest ? format(new Date(contest.starttime), "yyyy-MM-dd'T'HH:mm") : '',
            endtime: contest ? format(new Date(contest.endtime), "yyyy-MM-dd'T'HH:mm") : '',
            submit_start_time: contest?.submit_start_time ? format(new Date(contest.submit_start_time), "yyyy-MM-dd'T'HH:mm") : '',
            submit_end_time: contest?.submit_end_time ? format(new Date(contest.submit_end_time), "yyyy-MM-dd'T'HH:mm") : '',
            description: contest?.description || '',
            registration_mode: existingMode,
            registration_allowed_tags: existingTags,
        },
    });

    useEffect(() => {
        if (open) {
            const mode: RegistrationMode = (contest?.registration_config?.mode as RegistrationMode) || "auto";
            const tags = contest?.registration_config?.allowed_tags?.join(", ") || "";
            form.reset({
                id: contest?.id || '',
                name: contest?.name || '',
                starttime: contest ? format(new Date(contest.starttime), "yyyy-MM-dd'T'HH:mm") : '',
                endtime: contest ? format(new Date(contest.endtime), "yyyy-MM-dd'T'HH:mm") : '',
                submit_start_time: contest?.submit_start_time ? format(new Date(contest.submit_start_time), "yyyy-MM-dd'T'HH:mm") : '',
                submit_end_time: contest?.submit_end_time ? format(new Date(contest.submit_end_time), "yyyy-MM-dd'T'HH:mm") : '',
                description: contest?.description || '',
                registration_mode: mode,
                registration_allowed_tags: tags,
            });
        }
    }, [open, contest, form]);

    const watchedMode = form.watch("registration_mode");
    const showAllowedTags = watchedMode === "tag_auto" || watchedMode === "tag_review";


    const onSubmit = async (values: ContestFormValues) => {
        const registrationConfig: RegistrationConfig = {
            mode: values.registration_mode,
            allowed_tags: parseAllowedTags(values.registration_allowed_tags),
        };
        const payload = {
            id: values.id,
            name: values.name,
            starttime: new Date(values.starttime).toISOString(),
            endtime: new Date(values.endtime).toISOString(),
            submit_start_time: values.submit_start_time ? new Date(values.submit_start_time).toISOString() : null,
            submit_end_time: values.submit_end_time ? new Date(values.submit_end_time).toISOString() : null,
            description: values.description || '',
            registration_config: registrationConfig,
        };
        try {
            if (isEditing) {
                await api.put(`/admin/contests/${contest.id}`, payload);
            } else {
                await api.post('/admin/contests', payload);
            }
            toast({ title: `Contest ${isEditing ? 'Updated' : 'Created'}`, description: `Contest "${payload.name}" has been saved.` });
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
            <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
                <DialogHeader>
                    <DialogTitle>{isEditing ? "Edit Contest" : "Create New Contest"}</DialogTitle>
                    <DialogDescription>
                        {isEditing ? `Editing contest: ${contest.name}` : "Fill in the details for the new contest."}
                    </DialogDescription>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                        <FormField control={form.control} name="id" render={({ field }) => (
                            <FormItem><FormLabel>ID</FormLabel><FormControl><Input {...field} disabled={isEditing} /></FormControl><FormMessage /></FormItem>
                        )} />
                        <FormField control={form.control} name="name" render={({ field }) => (
                            <FormItem><FormLabel>Name</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>
                        )} />
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
                        <FormField control={form.control} name="description" render={({ field }) => (
                            <FormItem><FormLabel>Description (Markdown)</FormLabel><FormControl><Textarea {...field} rows={5} /></FormControl><FormMessage /></FormItem>
                        )} />
                        <div className="space-y-4 border-t pt-4">
                            <h3 className="text-sm font-semibold">Registration</h3>
                            <FormField control={form.control} name="registration_mode" render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Mode</FormLabel>
                                    <Select value={field.value} onValueChange={field.onChange}>
                                        <FormControl>
                                            <SelectTrigger><SelectValue placeholder="Select registration mode" /></SelectTrigger>
                                        </FormControl>
                                        <SelectContent>
                                            {(Object.keys(REGISTRATION_MODE_LABELS) as RegistrationMode[]).map((m) => (
                                                <SelectItem key={m} value={m}>{REGISTRATION_MODE_LABELS[m]}</SelectItem>
                                            ))}
                                        </SelectContent>
                                    </Select>
                                    <FormMessage />
                                </FormItem>
                            )} />
                            {showAllowedTags && (
                                <FormField control={form.control} name="registration_allowed_tags" render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Allowed Tags (comma-separated)</FormLabel>
                                        <FormControl>
                                            <Input {...field} placeholder="e.g. sct, vip, staff" />
                                        </FormControl>
                                        <FormMessage />
                                    </FormItem>
                                )} />
                            )}
                        </div>
                        <DialogFooter>
                            <Button type="submit" disabled={form.formState.isSubmitting}>{form.formState.isSubmitting ? "Saving..." : "Save"}</Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}

export function DeleteContestButton({ contest, onSuccess, trigger }: { contest: Contest, onSuccess: () => void, trigger: React.ReactNode }) {
    const { toast } = useToast();

    const handleDelete = async () => {
        try {
            await api.delete(`/admin/contests/${contest.id}`);
            toast({ title: "Contest Deleted", description: `Contest "${contest.name}" and all its problems have been deleted.` });
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
                    This action cannot be undone. This will permanently delete the contest `{contest.name}` and all associated problems and their files from the disk.
                </AlertDialogDescription>
                <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction onClick={handleDelete} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
                        Delete
                    </AlertDialogAction>
                </AlertDialogFooter>
            </AlertDialogContent>
        </AlertDialog>
    );
}
