"use client";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import { DevPodInstance } from "@/lib/types";
import { CopyButton } from "@/components/ui/shadcn-io/copy-button";
import { format } from "date-fns";
import { useTranslations } from "next-intl";

export function DevPodRow({ inst, onStart, onStop, onDelete }: {
  inst: DevPodInstance;
  onStart: () => void; onStop: () => void; onDelete: () => void;
}) {
  const t = useTranslations("devpods");
  const running = inst.phase === "Running";
  return (
    <TableRow>
      <TableCell className="font-mono text-xs">{inst.name}</TableCell>
      <TableCell>{inst.template}</TableCell>
      <TableCell>{inst.phase}</TableCell>
      <TableCell className="flex items-center gap-2">
        {inst.ssh_command ? (
          <>
            <code className="font-mono text-xs">{inst.ssh_command}</code>
            <CopyButton content={inst.ssh_command} size="sm" />
          </>
        ) : <span className="text-xs text-muted-foreground">{t("preparing")}</span>}
      </TableCell>
      <TableCell>{inst.created_at ? format(new Date(inst.created_at), "MM/dd HH:mm") : "-"}</TableCell>
      <TableCell className="space-x-1">
        {!running && <Button size="sm" variant="outline" onClick={onStart}>{t("start")}</Button>}
        {running && <Button size="sm" variant="outline" onClick={onStop}>{t("stop")}</Button>}
        <Button size="sm" variant="ghost" className="text-destructive" onClick={onDelete}>{t("delete")}</Button>
      </TableCell>
    </TableRow>
  );
}
