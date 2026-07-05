"use client";

import { useState, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useSWRConfig } from "swr";
import { Announcement } from "@/lib/types";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { Button, buttonVariants } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "../ui/textarea";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";

const announcementSchema = z.object({
    title: z.string().min(1, "Title is required"),
    description: z.string().min(1, "Description is required"),
});

type AnnouncementFormValues = z.infer<typeof announcementSchema>;

export function AnnouncementFormDialog({
    contestId,
    announcement,
    onSuccess,
    trigger
}: {
    contestId: string,
    announcement?: Announcement,
    onSuccess: () => void,
    trigger: React.ReactNode
}) {
    const [open, setOpen] = useState(false);
    const { toast } = useToast();
    const isEditing = !!announcement;

    const form = useForm<AnnouncementFormValues>({
        resolver: zodResolver(announcementSchema),
        defaultValues: {
            title: announcement?.title || '',
            description: announcement?.description || '',
        },
    });

    useEffect(() => {
        if (open) {
            form.reset({
                title: announcement?.title || '',
                description: announcement?.description || '',
            });
        }
    }, [open, announcement, form]);

    const onSubmit = async (values: AnnouncementFormValues) => {
        try {
            if (isEditing) {
                await api.put(`/admin/contests/${contestId}/announcements/${announcement.id}`, values);
            } else {
                await api.post(`/admin/contests/${contestId}/announcements`, values);
            }
            toast({ title: `Announcement ${isEditing ? 'Updated' : 'Created'}` });
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
                    <DialogTitle>{isEditing ? "Edit Announcement" : "Create New Announcement"}</DialogTitle>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                        <FormField control={form.control} name="title" render={({ field }) => (
                            <FormItem><FormLabel>Title</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>
                        )} />
                        <FormField control={form.control} name="description" render={({ field }) => (
                            <FormItem><FormLabel>Description (Markdown)</FormLabel><FormControl><Textarea {...field} rows={10} /></FormControl><FormMessage /></FormItem>
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

export function DeleteAnnouncementButton({ contestId, announcement, onSuccess, trigger }: { contestId: string, announcement: Announcement, onSuccess: () => void, trigger: React.ReactNode }) {
    const { toast } = useToast();

    const handleDelete = async () => {
        try {
            await api.delete(`/admin/contests/${contestId}/announcements/${announcement.id}`);
            toast({ title: "Announcement Deleted" });
            onSuccess();
        } catch (err: any) {
            toast({ variant: "destructive", title: "Delete Failed", description: err.response?.data?.message });
        }
    }

    return (
        <AlertDialog>
            <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>
            <AlertDialogContent>
                <AlertDialogHeader><AlertDialogTitle>Confirm Deletion</AlertDialogTitle></AlertDialogHeader>
                <AlertDialogDescription>
                    Are you sure you want to delete the announcement "{announcement.title}"? This action cannot be undone.
                </AlertDialogDescription>
                <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction onClick={handleDelete} className={buttonVariants({ variant: 'destructive'})}>
                        Delete
                    </AlertDialogAction>
                </AlertDialogFooter>
            </AlertDialogContent>
        </AlertDialog>
    );
}