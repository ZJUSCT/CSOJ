"use client";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import { DevPodInstance } from "@/lib/types";
import { CopyButton } from "@/components/ui/shadcn-io/copy-button";
import { format } from "date-fns";
import { useTranslations } from "next-intl";
import { Loader2 } from "lucide-react";
import { DevPodEventButton } from "@/components/devpods/devpod-event-dialog";

export type DevPodAction = "start" | "stop" | "delete";

export function DevPodRow({ inst, templateName, pendingAction, startDisabled, onStart, onStop, onDelete }: {
  inst: DevPodInstance;
  templateName?: string;
  pendingAction?: DevPodAction;
  startDisabled?: boolean;
  onStart: () => void; onStop: () => void; onDelete: () => void;
}) {
  const t = useTranslations("devpods");
  const running = inst.running;
  const showStop = pendingAction === "stop" || (pendingAction !== "start" && running);
  const stopped = inst.phase === "Stopped";
  const pendingLabel = pendingAction ? t(pendingAction === "start" ? "starting" : pendingAction === "stop" ? "stopping" : "deleting") : "";
  const sshPlaceholder = pendingAction
    ? pendingLabel
    : stopped
      ? t("stopped")
      : t("preparing");
  return (
    <TableRow className="h-12" aria-busy={!!pendingAction}>
      <TableCell className="font-mono text-xs">{inst.name}</TableCell>
      <TableCell>{templateName ?? inst.template}</TableCell>
      <TableCell className="w-28 whitespace-nowrap">
        <div className="flex h-6 items-center">
          {pendingAction ? (
            <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
              {pendingLabel}
            </span>
          ) : inst.phase}
        </div>
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          {inst.ssh_command && !pendingAction ? (
            <>
              <code className="font-mono text-xs leading-6">{inst.ssh_command}</code>
              <CopyButton content={inst.ssh_command} size="sm" aria-label="Copy SSH command" />
            </>
          ) : <span className="whitespace-nowrap text-xs text-muted-foreground leading-6">{sshPlaceholder}</span>}
        </div>
      </TableCell>
      <TableCell>{inst.created_at ? format(new Date(inst.created_at), "MM/dd HH:mm") : "-"}</TableCell>
      <TableCell className="whitespace-nowrap">
        <div className="flex items-center gap-1">
          {!showStop && (
            <Button className="w-24" size="sm" variant="outline" disabled={!!pendingAction || startDisabled} onClick={onStart}>
              {pendingAction === "start" && <Loader2 className="mr-1.5 h-3.5 w-3.5 shrink-0 animate-spin" />}
              {pendingAction === "start" ? t("starting") : t("start")}
            </Button>
          )}
          {showStop && (
            <Button className="w-24" size="sm" variant="outline" disabled={!!pendingAction} onClick={onStop}>
              {pendingAction === "stop" && <Loader2 className="mr-1.5 h-3.5 w-3.5 shrink-0 animate-spin" />}
              {pendingAction === "stop" ? t("stopping") : t("stop")}
            </Button>
          )}
          <DevPodEventButton endpoint={`/devpods/${encodeURIComponent(inst.name)}/events`} name={inst.name} disabled={pendingAction === "delete"} />
          <Button size="sm" variant="ghost" disabled={!!pendingAction} className="w-24 text-destructive" onClick={onDelete}>
            {pendingAction === "delete" && <Loader2 className="mr-1.5 h-3.5 w-3.5 shrink-0 animate-spin" />}
            {pendingAction === "delete" ? t("deleting") : t("delete")}
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}
