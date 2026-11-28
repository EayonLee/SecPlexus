"use client";

import { useQuery } from "@tanstack/react-query";
import { useFormatter, useLocale, useTranslations } from "next-intl";

import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";

import { listEventClasses } from "../api/ingest/event-schema";
import type { SampleResult } from "../api/ingest/schemas";

export function IngestConfirm({
  title,
  description,
  action,
  pending,
  error,
  destructive,
  disabled,
  onCancel,
  onConfirm,
}: {
  title: string;
  description: string;
  action: string;
  pending: boolean;
  error: boolean;
  destructive?: boolean;
  disabled?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const t = useTranslations("Ingest");
  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onCancel();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{t("operationFailed")}</AlertDescription>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t("cancel")}</AlertDialogCancel>
          <Button variant={destructive ? "destructive" : "default"} disabled={pending || disabled} onClick={onConfirm}>
            {action}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function MappingPreview({ result }: { result: SampleResult }) {
  const t = useTranslations("Mappings");
  const format = useFormatter();
  const mixed = useTranslations("MixedRule");
  const locale = useLocale();
  const catalog = useQuery({
    queryKey: ["ingest", "event-schema"],
    queryFn: ({ signal }) => listEventClasses(signal),
    enabled: !!result.previews?.length,
  });
  return (
    <section className="flex min-w-0 flex-col gap-3" aria-label={t("preview")} aria-live="polite">
      <p className="text-sm font-medium">{mixed("matchedSamples", { matched: result.matched, total: result.total })}</p>
      {result.compile_error && (
        <Alert variant="destructive">
          <AlertDescription>{result.compile_error}</AlertDescription>
        </Alert>
      )}
      {result.failures?.map((item) => (
        <p key={item.index} className="text-destructive text-sm break-words">
          {t("sampleNumber", { number: item.index + 1 })} · {item.reason}
        </p>
      ))}
      {result.previews?.map((item, index) => (
        <details key={item.index} open={index === 0} className="min-w-0 rounded-lg border">
          <summary className="cursor-pointer px-3 py-2 text-sm font-medium">
            {t("sampleNumber", { number: item.index + 1 })}

            <span className="text-muted-foreground ml-2 text-xs">
              {[
                mixed(item.status),
                catalog.data?.classes.find((event) => event.class_uid === item.event.class_uid)?.name[locale],
              ]
                .filter(Boolean)
                .join(" · ")}
            </span>
          </summary>
          <div className="flex min-w-0 flex-col gap-2 px-3 pb-3">
            <p className="text-muted-foreground text-xs break-words">
              {t("partitionKey", { key: item.partition_key || t("eventIdentifier") })}
              {typeof item.event.severity_id === "number" && (
                <span className="ml-2">{t("severity", { level: format.number(item.event.severity_id) })}</span>
              )}
            </p>
            <dl className="divide-y rounded-md border px-3">
              {["activity_id", "src_endpoint", "dst_endpoint", "user", "unmapped"]
                .filter((key) => item.event[key] != null)
                .map((key) => (
                  <div
                    key={key}
                    className="grid min-w-0 grid-cols-[minmax(5rem,0.75fr)_minmax(0,1.3fr)] gap-3 py-2 font-mono text-xs"
                  >
                    <dt className="text-muted-foreground break-all">{key}</dt>
                    <dd className="min-w-0 break-all">{JSON.stringify(item.event[key])}</dd>
                  </div>
                ))}
            </dl>
            <details>
              <summary className="text-primary cursor-pointer py-1 text-sm font-medium">{t("fullEvent")}</summary>
              <pre className="bg-muted max-h-72 overflow-auto rounded-md p-3 text-xs">
                {JSON.stringify(item.event, null, 2)}
              </pre>
            </details>
          </div>
        </details>
      ))}
    </section>
  );
}
