"use client";
import { useState, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { useToast } from "@/hooks/use-toast";
import api from "@/lib/api";
import { User } from "@/lib/types";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { Textarea } from "../ui/textarea";
import { Separator } from "../ui/separator";
import { format } from "date-fns";
import { Checkbox } from "@/components/ui/checkbox";

// --- Create User ---
const createUserSchema = z.object({
	username: z.string().min(1, "Username is required"),
	nickname: z.string().min(1, "Nickname is required"),
	password: z.string().min(6, "Password must be at least 6 characters"),
	disable_rank: z.boolean().default(false).optional(),
	tags: z.string().optional(),
});

export function CreateUserDialog({ onUserCreated }: { onUserCreated: () => void }) {
	const [open, setOpen] = useState(false);
	const { toast } = useToast();
	const form = useForm<z.infer<typeof createUserSchema>>({ resolver: zodResolver(createUserSchema), defaultValues: { username: '', nickname: '', password: '', disable_rank: false } });

	const onSubmit = async (values: z.infer<typeof createUserSchema> & { tags?: string }) => {
		try {
			await api.post('/admin/users', values);
			toast({ title: "User Created", description: `User ${values.username} has been created.` });
			onUserCreated();
			setOpen(false);
			form.reset();
		} catch (err: any) {
			toast({ variant: "destructive", title: "Creation Failed", description: err.response?.data?.message });
		}
	};

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild><Button>Create User</Button></DialogTrigger>
			<DialogContent>
				<DialogHeader><DialogTitle>Create New User</DialogTitle></DialogHeader>
				<Form {...form}>
					<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
						<FormField control={form.control} name="username" render={({ field }) => (<FormItem><FormLabel>Username</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
						<FormField control={form.control} name="nickname" render={({ field }) => (<FormItem><FormLabel>Nickname</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
						<FormField control={form.control} name="password" render={({ field }) => (<FormItem><FormLabel>Password</FormLabel><FormControl><Input type="password" {...field} /></FormControl><FormMessage /></FormItem>)} />
						<FormField control={form.control} name="tags" render={({ field }) => (<FormItem><FormLabel>Tags</FormLabel><FormControl><Input placeholder="tag1,tag2,tag3" {...field} /></FormControl><FormMessage /></FormItem>)} />
						<FormField
							control={form.control}
							name="disable_rank"
							render={({ field }) => (
								<FormItem className="flex flex-row items-center space-x-3 space-y-0 rounded-md border p-4">
									<FormControl>
										<Checkbox
											checked={field.value}
											onCheckedChange={field.onChange}
										/>
									</FormControl>
									<div className="space-y-1 leading-none">
										<FormLabel>Disable Ranking</FormLabel>
										<FormDescription>If checked, this user will not affect other users' rankings.</FormDescription>
									</div>
								</FormItem>
							)} />
						<DialogFooter><Button type="submit" disabled={form.formState.isSubmitting}>{form.formState.isSubmitting ? "Creating..." : "Create"}</Button></DialogFooter>
					</form>
				</Form>
			</DialogContent>
		</Dialog>
	);
}

// --- Edit User ---
const editUserSchema = z.object({
	nickname: z.string().min(1, "Nickname is required"),
	signature: z.string().optional(),
    ban_reason: z.string().optional(),
    banned_until: z.string().optional(),
    disable_rank: z.boolean().optional(),
    tags: z.string().optional(),
});
export function EditUserDialog({ user, onUserUpdated, trigger }: { user: User, onUserUpdated: () => void, trigger: React.ReactNode }) {
	const [open, setOpen] = useState(false);
	const { toast } = useToast();
	const form = useForm<z.infer<typeof editUserSchema>>({
		resolver: zodResolver(editUserSchema),
		defaultValues: {
            nickname: user.nickname,
            signature: user.signature || '',
            ban_reason: user.ban_reason || '',
            banned_until: user.banned_until ? format(new Date(user.banned_until), "yyyy-MM-dd'T'HH:mm") : '',
            disable_rank: user.disable_rank,
            tags: user.tags || '',
        },
	});

    useEffect(() => {
        if (open) {
            form.reset({
                nickname: user.nickname,
                signature: user.signature || '',
                ban_reason: user.ban_reason || '',
                banned_until: user.banned_until ? format(new Date(user.banned_until), "yyyy-MM-dd'T'HH:mm") : '',
                disable_rank: user.disable_rank,
                tags: user.tags || '',
            });
        }
    }, [open, user, form]);

	const onSubmit = async (values: z.infer<typeof editUserSchema>) => {
        const payload: any = {
            nickname: values.nickname,
            signature: values.signature,
            ban_reason: values.ban_reason,
            disable_rank: values.disable_rank,
            tags: values.tags,
        };
        // Handle time: convert local time to ISO string for backend, or send empty string to unban
        if (values.banned_until) {
            payload.banned_until = new Date(values.banned_until).toISOString();
        } else {
            payload.banned_until = ""; // Signal to backend to unban
        }

		try {
			await api.patch(`/admin/users/${user.id}`, payload);
			toast({ title: "User Updated", description: "User profile has been updated." });
			onUserUpdated();
			setOpen(false);
		} catch (err: any) {
			toast({ variant: "destructive", title: "Update Failed", description: err.response?.data?.message });
		}
	};

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>{trigger}</DialogTrigger>
			<DialogContent>
				<DialogHeader><DialogTitle>Edit Profile for @{user.username}</DialogTitle></DialogHeader>
				<Form {...form}>
					<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
						<FormField control={form.control} name="nickname" render={({ field }) => (<FormItem><FormLabel>Nickname</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
						<FormField control={form.control} name="signature" render={({ field }) => (<FormItem><FormLabel>Signature</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
						<FormField control={form.control} name="tags" render={({ field }) => (<FormItem><FormLabel>Tags</FormLabel><FormControl><Input placeholder="tag1,tag2" {...field} /></FormControl><FormMessage /></FormItem>)} />
						<FormField
							control={form.control}
							name="disable_rank"
							render={({ field }) => (
								<FormItem className="flex flex-row items-center space-x-3 space-y-0 rounded-md border p-4">
									<FormControl>
										<Checkbox
											checked={field.value}
											onCheckedChange={field.onChange} />
									</FormControl>
									<div className="space-y-1 leading-none">
										<FormLabel>Disable Ranking</FormLabel>
										<FormDescription>If checked, this user will appear in leaderboards but will not affect other users' rankings.</FormDescription>
									</div>
								</FormItem>
							)} />

                        <Separator className="my-4" />
                        <h3 className="text-lg font-semibold">Ban Controls</h3>
                        <p className="text-sm text-muted-foreground">To unban a user, clear the "Banned Until" date.</p>
                        <FormField control={form.control} name="banned_until" render={({ field }) => (
                            <FormItem>
                                <FormLabel>Banned Until</FormLabel>
                                <FormControl><Input type="datetime-local" {...field} /></FormControl>
                                <FormMessage />
                            </FormItem>
                        )} />
                        <FormField control={form.control} name="ban_reason" render={({ field }) => (
                            <FormItem>
                                <FormLabel>Ban Reason</FormLabel>
                                <FormControl><Textarea placeholder="Enter the reason for the ban..." {...field} /></FormControl>
                                <FormMessage />
                            </FormItem>
                        )} />

						<DialogFooter><Button type="submit" disabled={form.formState.isSubmitting}>{form.formState.isSubmitting ? "Saving..." : "Save Changes"}</Button></DialogFooter>
					</form>
				</Form>
			</DialogContent>
		</Dialog>
	);
}


// --- Reset Password ---
const resetPasswordSchema = z.object({ password: z.string().min(6, "Password must be at least 6 characters") });
export function ResetPasswordDialog({ userId, trigger }: { userId: string, trigger: React.ReactNode }) {
	const [open, setOpen] = useState(false);
	const { toast } = useToast();
	const form = useForm<z.infer<typeof resetPasswordSchema>>({ resolver: zodResolver(resetPasswordSchema), defaultValues: { password: '' } });

	const onSubmit = async (values: z.infer<typeof resetPasswordSchema>) => {
		try {
			await api.post(`/admin/users/${userId}/reset-password`, values);
			toast({ title: "Password Reset", description: "User's password has been successfully reset." });
			setOpen(false);
			form.reset();
		} catch (err: any) {
			toast({ variant: "destructive", title: "Reset Failed", description: err.response?.data?.message });
		}
	};

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>{trigger}</DialogTrigger>
			<DialogContent>
				<DialogHeader><DialogTitle>Reset Password</DialogTitle></DialogHeader>
				<Form {...form}>
					<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
						<FormField control={form.control} name="password" render={({ field }) => (<FormItem><FormLabel>New Password</FormLabel><FormControl><Input type="password" {...field} /></FormControl><FormMessage /></FormItem>)} />
						<DialogFooter><Button type="submit" disabled={form.formState.isSubmitting}>Set New Password</Button></DialogFooter>
					</form>
				</Form>
			</DialogContent>
		</Dialog>
	);
}

// --- Register for Contest ---
const registerContestSchema = z.object({ contest_id: z.string().min(1, "Contest ID is required") });
export function RegisterContestDialog({ userId, trigger }: { userId: string, trigger: React.ReactNode }) {
	const [open, setOpen] = useState(false);
	const { toast } = useToast();
	const form = useForm<z.infer<typeof registerContestSchema>>({ resolver: zodResolver(registerContestSchema), defaultValues: { contest_id: '' } });

	const onSubmit = async (values: z.infer<typeof registerContestSchema>) => {
		try {
			await api.post(`/admin/users/${userId}/register-contest`, values);
			toast({ title: "Registration Successful", description: `User has been registered for contest ${values.contest_id}.` });
			setOpen(false);
			form.reset();
		} catch (err: any) {
			toast({ variant: "destructive", title: "Registration Failed", description: err.response?.data?.message });
		}
	};
	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>{trigger}</DialogTrigger>
			<DialogContent>
				<DialogHeader><DialogTitle>Register User for Contest</DialogTitle></DialogHeader>
				<Form {...form}>
					<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
						<FormField control={form.control} name="contest_id" render={({ field }) => (<FormItem><FormLabel>Contest ID</FormLabel><FormControl><Input {...field} /></FormControl><FormMessage /></FormItem>)} />
						<DialogFooter><Button type="submit" disabled={form.formState.isSubmitting}>Register</Button></DialogFooter>
					</form>
				</Form>
			</DialogContent>
		</Dialog>
	);
}

// --- Delete User ---
export function DeleteUserMenuItem({ userId, onUserDeleted }: { userId: string, onUserDeleted: () => void }) {
	const { toast } = useToast();
	const handleDelete = async () => {
		if (confirm("Are you sure you want to delete this user? This action is irreversible.")) {
			try {
				await api.delete(`/admin/users/${userId}`);
				toast({ title: "User Deleted" });
				onUserDeleted();
			} catch (err: any) {
				toast({ variant: "destructive", title: "Deletion Failed", description: err.response?.data?.message });
			}
		}
	};
	return <DropdownMenuItem className="text-destructive" onClick={handleDelete}>Delete User</DropdownMenuItem>;
}