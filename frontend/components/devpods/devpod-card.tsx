"use client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DevPodTemplate } from "@/lib/types";
import { useTranslations } from "next-intl";

export function DevPodCard({ tpl, used, disabled, onCreate }: {
  tpl: DevPodTemplate; used: number; disabled: boolean; onCreate: () => void;
}) {
  const t = useTranslations("devpods");
  const atLimit = used >= tpl.default_per_user;
  return (
    <Card>
      <CardHeader><CardTitle>{tpl.name}</CardTitle></CardHeader>
      <CardContent className="space-y-2 text-sm text-muted-foreground">
        <div>{tpl.cores} {t("cores")} · {(tpl.memory / (1<<30)).toFixed(0)}Gi</div>
        {tpl.node_selector && Object.keys(tpl.node_selector).length > 0 && (
          <div className="font-mono text-xs">{Object.entries(tpl.node_selector).map(([k,v])=>`${k}=${v}`).join(", ")}</div>
        )}
        <div>{t("used")}: {used}/{tpl.default_per_user}</div>
        <Button className="w-full" disabled={disabled || atLimit} onClick={onSubmit(onCreate)}>
          {atLimit ? t("limitReached") : t("create")}
        </Button>
      </CardContent>
    </Card>
  );
}
function onSubmit(fn: () => void) { return () => fn(); }
