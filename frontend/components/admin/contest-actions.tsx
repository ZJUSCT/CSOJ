"use client";

import { useState, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useSWRConfig } from "swr";
import { Contest } from "@/lib/types";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "../ui/textarea";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { format } from "date-fns";

const contestSchema = z.object({
    id: z.string().min(1, "ID is required").regex(/^[a-z0-9-_]+$/, "ID must be lowercase alphanumeric with hyphens"),
    name: z.string().min(1, "Name is required"),
    starttime: z.string().refine((val) => !isNaN(Date.parse(val)), "Invalid start time"),
    endtime: z.string().refine((val) => !isNaN(Date.parse(val)), "Invalid end time"),
    description: z.string().optional(),
});

type ContestFormValues = z.infer<typeof contestSchema>;

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

    const form = useForm<ContestFormValues>({
        resolver: zodResolver(contestSchema),
        defaultValues: {
            id: contest?.id || '',
            name: contest?.name || '',
            starttime: contest ? format(new Date(contest.starttime), "yyyy-MM-dd'T'HH:mm") : '',
            endtime: contest ? format(new Date(contest.endtime), "yyyy-MM-dd'T'HH:mm") : '',
            description: contest?.description || '',
        },
    });

    useEffect(() => {
        if (open) {
            form.reset({
                id: contest?.id || '',
                name: contest?.name || '',
                starttime: contest ? format(new Date(contest.starttime), "yyyy-MM-dd'T'HH:mm") : '',
                endtime: contest ? format(new Date(contest.endtime), "yyyy-MM-dd'T'HH:mm") : '',
                description: contest?.description || '',
            });
        }
    }, [open, contest, form]);


    const onSubmit = async (values: ContestFormValues) => {
        const payload = {
            ...values,
            starttime: new Date(values.starttime).toISOString(),
            endtime: new Date(values.endtime).toISOString(),
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
            <DialogContent>
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
                        <FormField control={form.control} name="starttime" render={({ field }) => (
                            <FormItem><FormLabel>Start Time</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>
                        )} />
                        <FormField control={form.control} name="endtime" render={({ field }) => (
                            <FormItem><FormLabel>End Time</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>
                        )} />
                        <FormField control={form.control} name="description" render={({ field }) => (
                            <FormItem><FormLabel>Description (Markdown)</FormLabel><FormControl><Textarea {...field} rows={5} /></FormControl><FormMessage /></FormItem>
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