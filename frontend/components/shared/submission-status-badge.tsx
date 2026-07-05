"use client";
import { Badge } from "@/components/ui/badge";
import { Status } from "@/lib/types";
import { useTranslations } from "next-intl";

interface SubmissionStatusBadgeProps {
  status: Status;
}

export default function SubmissionStatusBadge({ status }: SubmissionStatusBadgeProps) {
  const t = useTranslations('submissions.status');

  const statusStyles: Record<Status, string> = {
    Queued: "bg-blue-600 hover:bg-blue-700 text-white",
    Running: "bg-amber-500 hover:bg-amber-600 text-white animate-pulse",
    Success: "bg-green-600 hover:bg-green-700 text-white",
    Failed: "bg-red-600 hover:bg-red-700 text-white",
  };

  const translatedStatus = t(status as 'Queued' | 'Running' | 'Success' | 'Failed');

  return (
    <Badge 
      className={`uppercase font-semibold tracking-wider ${statusStyles[status]}`}
    >
      {translatedStatus}
    </Badge>
  );
}