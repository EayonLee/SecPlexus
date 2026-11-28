"use client";

import { useState } from "react";

import { useInfiniteQuery, useMutation, useQuery } from "@tanstack/react-query";
import { useFormatter, useTranslations } from "next-intl";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "cn";
import {
  getMappingRevision,
  listMappingRevisions,
  mappingKey,
  restoreMappingRevision,
} from "@/spx/api/ingest/mappings";
import type { ApiMappingDetail } from "@/spx/api/ingest/schemas";
import { IngestConfirm } from "@/spx/components/ingest-feedback";
import { SheetPanel } from "@/spx/components/sheet-panel";

export function RuleRevisions({
  mapping,
  canRestore,
  inputCount,
  onClose,
  onRestore,
}: {
  mapping: ApiMappingDetail;
  canRestore: boolean;
  inputCount: number;
  onClose: () => void;
  onRestore: (saved: ApiMappingDetail) => void;
}) {
  const t = useTranslations("RuleWizard");
  const shared = useTranslations("Ingest");
  const format = useFormatter();
  const [selected, setSelected] = useState(mapping.version);
  const [confirm, setConfirm] = useState(false);
  const list = useInfiniteQuery({
    queryKey: [...mappingKey, mapping.id, "revisions"],
    queryFn: ({ pageParam, signal }) => listMappingRevisions(mapping.id, pageParam, signal),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (page) => page.next_before_version,
  });
  const detail = useQuery({
    queryKey: [...mappingKey, mapping.id, "revision", selected],
    queryFn: ({ signal }) => getMappingRevision(mapping.id, selected, signal),
  });
  const restore = useMutation({
    mutationFn: () => restoreMappingRevision(mapping.id, selected, mapping.version),
    onSuccess: onRestore,
  });

  const rows = list.data?.pages.flatMap((page) => page.items);
  const value = detail.data;
  const diff: { name: string; previous: unknown; current: unknown }[] = [];
  if (value && selected !== mapping.version) {
    const definition = mapping.definition as Record<string, unknown>;
    for (const [name, previous, current] of [
      [t("revisionName"), value.name, mapping.name],
      [t("revisionVendor"), value.vendor, mapping.vendor],
      [t("revisionProduct"), value.product, mapping.product],
      ...Array.from(new Set([...Object.keys(value.definition ?? {}), ...Object.keys(definition)]), (name) => [
        name,
        value.definition?.[name],
        definition[name],
      ]),
    ] as [string, unknown, unknown][]) {
      if (JSON.stringify(previous) !== JSON.stringify(current)) diff.push({ name, previous, current });
    }
  }
  return (
    <SheetPanel
      layout="management"
      title={t("revisionHistory")}
      onClose={restore.isPending ? () => undefined : onClose}
      footer={
        canRestore &&
        selected !== mapping.version &&
        value?.verification && (
          <Button size="sm" onClick={() => setConfirm(true)}>
            {t("restoreRevision")}
          </Button>
        )
      }
    >
      <div className="grid min-h-full min-w-0 @container">
        <div className="grid min-w-0 grid-cols-1 @min-[37rem]:grid-cols-[12rem_minmax(0,1fr)]">
          <nav
            aria-label={t("revisionHistory")}
            className="min-w-0 border-b py-3 @min-[37rem]:border-e @min-[37rem]:border-b-0"
          >
            {list.isPending && <Skeleton className="mx-4 h-16" />}
            {list.isError && (
              <Button variant="outline" size="sm" className="ms-4" onClick={() => void list.refetch()}>
                {shared("retry")}
              </Button>
            )}
            {!list.isPending && !list.isError && !rows?.length && (
              <p className="px-4 text-sm text-muted-foreground">{t("noRevisions")}</p>
            )}
            {rows?.map((row) => (
              <Button
                variant="ghost"
                key={row.version}
                type="button"
                aria-current={selected === row.version}
                className={cn(
                  "grid h-auto w-full justify-items-start gap-0 rounded-none border-y border-transparent px-4 py-2.5 text-start text-sm hover:bg-muted/60",
                  selected === row.version && "border-border bg-muted/70",
                )}
                onClick={() => setSelected(row.version)}
              >
                <span className="block font-medium">{t("revisionNumber", { version: row.version })}</span>
                <time dateTime={row.recorded_at} className="block text-xs text-muted-foreground tabular-nums">
                  {format.dateTime(new Date(row.recorded_at), { dateStyle: "short", timeStyle: "short" })}
                </time>
              </Button>
            ))}
            {list.hasNextPage && (
              <Button
                className="mx-3 mt-2"
                variant="ghost"
                size="sm"
                disabled={list.isFetchingNextPage}
                onClick={() => void list.fetchNextPage()}
              >
                {t("olderRevisions")}
              </Button>
            )}
          </nav>
          <div className="grid min-w-0 content-start gap-4 p-4 sm:p-6">
            {detail.isPending && <Skeleton className="h-24" />}
            {detail.isError && (
              <Alert variant="destructive">
                <AlertDescription className="flex flex-wrap items-center gap-2">
                  {shared("operationFailed")}
                  <Button variant="outline" size="sm" onClick={() => void detail.refetch()}>
                    {shared("retry")}
                  </Button>
                </AlertDescription>
              </Alert>
            )}
            {value && (
              <>
                <div className="border-b pb-3">
                  <h3 className="font-medium">
                    {t("revisionNumber", { version: value.version })}
                    {value.version === mapping.version && (
                      <span className="ms-2 text-sm text-muted-foreground">{t("currentRevision")}</span>
                    )}
                  </h3>
                  {value.verification && (
                    <p className="mt-1 text-sm text-muted-foreground">
                      {t("verifiedSamples", {
                        passed: value.verification.passed,
                        total: value.verification.sample_count,
                      })}
                    </p>
                  )}
                </div>
                {diff.length > 0 ? (
                  <div className="min-w-0 divide-y rounded-md border">
                    {diff.map(({ name, previous, current }) => (
                      <details key={name} className="px-3 py-2.5 text-sm">
                        <summary className="cursor-pointer break-words font-medium">{name}</summary>
                        <div className="mt-2 grid min-w-0 gap-2">
                          {[
                            { title: t("revisionNumber", { version: value.version }), data: previous },
                            { title: t("currentRevision"), data: current },
                          ].map(({ title, data }) => (
                            <div key={title} className="min-w-0 space-y-1">
                              <p className="text-xs text-muted-foreground">{title}</p>
                              <pre className="max-h-48 overflow-auto rounded-md bg-muted/40 p-2 text-xs">
                                {data === undefined ? "" : JSON.stringify(data, null, 2)}
                              </pre>
                            </div>
                          ))}
                        </div>
                      </details>
                    ))}
                  </div>
                ) : selected !== mapping.version ? (
                  <p className="text-sm text-muted-foreground">{t("noRevisionChanges")}</p>
                ) : null}
              </>
            )}
          </div>
        </div>
      </div>
      {confirm && (
        <IngestConfirm
          title={t("restoreRevision")}
          description={t("restoreRevisionImpact", { version: selected, count: inputCount })}
          action={t("restoreRevision")}
          pending={restore.isPending}
          error={restore.isError}
          onCancel={() => setConfirm(false)}
          onConfirm={() => restore.mutate()}
        />
      )}
    </SheetPanel>
  );
}
