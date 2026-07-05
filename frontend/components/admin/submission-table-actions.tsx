"use client";

import { Submission } from "@/lib/types";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { Button } from "@/components/ui/button";
import {
    RefreshCcw,
    Download,
    XCircle,
    Ban,
    CheckCircle,
    Trash2,
    Edit,
} from "lucide-react";
import Link from "next/link";
import { UpdateSubmissionDialog } from "./submission-actions";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

interface SubmissionTableActionsProps {
    submission: Submission;
    mutate: () => void;
}

export function SubmissionTableActions({ submission, mutate }: SubmissionTableActionsProps) {
    const { toast } = useToast();

    const handleAction = async (action: 'rejudge' | 'interrupt' | 'delete' | 'validity', payload?: any) => {
		const endpoint = action === 'validity' ? `/admin/submissions/${submission.id}/validity` : `/admin/submissions/${submission.id}/${action}`;
		const method = action === 'validity' ? 'patch' : 'post';

		try {
            const apiCall = action === 'delete' ? api.delete(endpoint) : api[method](endpoint, payload);
			await apiCall;
			toast({ title: 'Success', description: `Action '${action}' for submission ${submission.id.substring(0,8)} completed.` });
			mutate();
		} catch (err: any) {
			toast({ variant: 'destructive', title: 'Error', description: err.response?.data?.message || `Failed to perform action '${action}'.` });
		}
	}

    return (
        <TooltipProvider>
            <div className="flex items-center justify-end gap-0.5">
                <Tooltip>
                    <UpdateSubmissionDialog submission={submission} onSubmissionUpdated={mutate}>
                        <TooltipTrigger asChild>
                            <Button variant="ghost" size="icon" className="h-8 w-8">
                                <Edit className="h-4 w-4" />
                            </Button>
                        </TooltipTrigger>
                    </UpdateSubmissionDialog>
                    <TooltipContent><p>Manual Override</p></TooltipContent>
                </Tooltip>

                <Tooltip>
                    <TooltipTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => handleAction('rejudge')}>
                            <RefreshCcw className="h-4 w-4" />
                        </Button>
                    </TooltipTrigger>
                    <TooltipContent><p>Rejudge</p></TooltipContent>
                </Tooltip>

                <Tooltip>
                    <TooltipTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8" asChild>
                             <Link href={`/api/v1/admin/submissions/${submission.id}/content`} download>
								<Download className="h-4 w-4" />
							</Link>
                        </Button>
                    </TooltipTrigger>
                    <TooltipContent><p>Download Files</p></TooltipContent>
                </Tooltip>

                <Tooltip>
                    <TooltipTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive hover:text-destructive" onClick={() => handleAction('interrupt')} disabled={submission.status !== 'Queued' && submission.status !== 'Running'}>
                            <XCircle className="h-4 w-4" />
                        </Button>
                    </TooltipTrigger>
                    <TooltipContent><p>Interrupt</p></TooltipContent>
                </Tooltip>

                <Tooltip>
                    <TooltipTrigger asChild>
                       {submission.is_valid ? (
                            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => handleAction('validity', { is_valid: false })}>
                                <Ban className="h-4 w-4" />
                            </Button>
                       ) : (
                            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => handleAction('validity', { is_valid: true })}>
                                <CheckCircle className="h-4 w-4" />
                            </Button>
                       )}
                    </TooltipTrigger>
                    <TooltipContent><p>{submission.is_valid ? 'Mark Invalid' : 'Mark Valid'}</p></TooltipContent>
                </Tooltip>

                <Tooltip>
                    <TooltipTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive hover:text-destructive" onClick={() => { if (confirm('Are you sure? This will delete the submission and its content permanently.')) handleAction('delete') }}>
                            <Trash2 className="h-4 w-4" />
                        </Button>
                    </TooltipTrigger>
                    <TooltipContent><p>Delete</p></TooltipContent>
                </Tooltip>
            </div>
        </TooltipProvider>
    );
}