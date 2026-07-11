"use client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DevPodTemplate } from "@/lib/types";
import { useTranslations } from "next-intl";
import { CircuitBoard, Cpu, MemoryStick, Server } from "lucide-react";

export function DevPodCard({ tpl, used, disabled, globalAtLimit, onCreate }: {
  tpl: DevPodTemplate; used: number; disabled: boolean; globalAtLimit?: boolean; onCreate: () => void;
}) {
  const t = useTranslations("devpods");
  const atLimit = used >= tpl.default_per_user;
  return (
    <Card className="flex h-full flex-col overflow-hidden">
      <CardHeader className="space-y-1 pb-4">
        <CardTitle className="truncate text-lg" title={tpl.name}>{tpl.name}</CardTitle>
        <CardDescription className="flex items-center gap-1.5">
          <Server className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{tpl.cluster_name}</span>
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-4">
        <div className={tpl.gpu_count > 0 ? "grid grid-cols-3 gap-2" : "grid grid-cols-2 gap-2"}>
          <div className="rounded-md border bg-muted/30 px-3 py-2.5">
            <div className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
              <Cpu className="h-3.5 w-3.5" />
              {t("cpu")}
            </div>
            <div className="font-medium">{tpl.cores} {t("cores")}</div>
          </div>
          <div className="rounded-md border bg-muted/30 px-3 py-2.5">
            <div className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
              <MemoryStick className="h-3.5 w-3.5" />
              {t("memory")}
            </div>
            <div className="font-medium">{(tpl.memory / (1 << 30)).toFixed(0)} GiB</div>
          </div>
          {tpl.gpu_count > 0 && (
            <div className="rounded-md border bg-muted/30 px-3 py-2.5">
              <div className="mb-1 flex items-center gap-1.5 whitespace-nowrap text-xs text-muted-foreground">
                <CircuitBoard className="h-3.5 w-3.5 shrink-0" />
                {t("gpu")}
              </div>
              <div className="whitespace-nowrap font-medium">{tpl.gpu_count}</div>
            </div>
          )}
        </div>

        <div className="mt-auto space-y-2.5 pt-1">
          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground">{t("usage")}</span>
            <span className="tabular-nums font-medium">{used} / {tpl.default_per_user}</span>
          </div>
          <Button className="w-full" disabled={disabled || atLimit || globalAtLimit} onClick={onCreate}>
            {atLimit || globalAtLimit ? t("limitReached") : t("create")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
