"use client";

import React, { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Submission } from "@/lib/types";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

const updateSubmissionSchema = z.object({
    status: z.enum(["Queued", "Running", "Success", "Failed"]).optional(),
    score: z.coerce.number().int().optional(),
    performance: z.coerce.number().optional(),
    info: z.string().optional().refine((val) => {
        if (val === "" || val === undefined) return true;
        try {
            JSON.parse(val);
            return true;
        } catch {
            return false;
        }
    }, { message: "Must be valid JSON or empty" }),
});


export function UpdateSubmissionDialog({
    submission,
    onSubmissionUpdated,
    children, // The trigger element
}: {
    submission: Submission,
    onSubmissionUpdated: () => void,
    children: React.ReactNode,
}) {
    const [open, setOpen] = useState(false);
    const { toast } = useToast();
    const form = useForm<z.infer<typeof updateSubmissionSchema>>({
        resolver: zodResolver(updateSubmissionSchema),
        defaultValues: {
            status: submission.status,
            score: submission.score,
            performance: submission.performance,
            info: JSON.stringify(submission.info, null, 2),
        }
    });

    const onSubmit = async (values: z.infer<typeof updateSubmissionSchema>) => {
        const payload: any = {};
        if (values.status) payload.status = values.status;
        if (values.score !== undefined) payload.score = values.score;
        if (values.performance !== undefined) payload.performance = values.performance;

        if (values.info) {
             try {
                payload.info = JSON.parse(values.info);
            } catch (e) {
                form.setError("info", { type: "manual", message: "Invalid JSON format." });
                return;
            }
        } else {
            payload.info = {};
        }

        if (Object.keys(payload).length === 0) {
            toast({ variant: "destructive", title: "No changes", description: "You must provide at least one field to update." });
            return;
        }

        try {
            await api.patch(`/admin/submissions/${submission.id}`, payload);
            toast({ title: "Submission Updated", description: "The submission has been manually updated. Note: This does not trigger automatic score recalculation." });
            onSubmissionUpdated();
            setOpen(false);
        } catch (err: any) {
            toast({ variant: "destructive", title: "Update Failed", description: err.response?.data?.message });
        }
    }

    React.useEffect(() => {
        if (open) {
            form.reset({
                status: submission.status,
                score: submission.score,
                performance: submission.performance,
                info: JSON.stringify(submission.info, null, 2),
            });
        }
    }, [open, submission, form]);

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
                {children}
            </DialogTrigger>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Manual Submission Override</DialogTitle>
                    <p className="text-sm text-destructive">Warning: This is a dangerous action that directly modifies the database. Use with caution.</p>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                        <FormField
                            control={form.control}
                            name="status"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Status</FormLabel>
                                    <Select onValueChange={field.onChange} defaultValue={field.value}>
                                        <FormControl><SelectTrigger><SelectValue placeholder="Select a status" /></SelectTrigger></FormControl>
                                        <SelectContent>
                                            <SelectItem value="Queued">Queued</SelectItem>
                                            <SelectItem value="Running">Running</SelectItem>
                                            <SelectItem value="Success">Success</SelectItem>
                                            <SelectItem value="Failed">Failed</SelectItem>
                                        </SelectContent>
                                    </Select>
                                </FormItem>
                            )} />

                        <FormField
                            control={form.control}
                            name="score"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Score</FormLabel>
                                    <FormControl><Input type="number" {...field} /></FormControl>
                                </FormItem>
                            )} />

                        <FormField
                            control={form.control}
                            name="performance"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Performance</FormLabel>
                                    <FormControl><Input type="number" step="any" {...field} /></FormControl> {/* Use step="any" for floats */}
                                    <FormMessage />
                                </FormItem>
                            )} />

                        <FormField
                            control={form.control}
                            name="info"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Info (JSON)</FormLabel>
                                    <FormControl><Textarea className="font-mono min-h-32" {...field} /></FormControl>
                                    <FormMessage />
                                </FormItem>
                            )} />
                        <DialogFooter>
                            <Button type="submit" variant="destructive" disabled={form.formState.isSubmitting}>
                                {form.formState.isSubmitting ? "Saving..." : "Save Changes"}
                            </Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    )
}