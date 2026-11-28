"use client";

import { useState } from "react";

import Link from "next/link";
import { notFound, useRouter } from "next/navigation";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MoreHorizontalIcon } from "lucide-react";
import { useFormatter, useLocale, useTranslations } from "next-intl";

import { permissions } from "@/auth/permissions.gen";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { APIError } from "@/spx/api/client";
import { listEventClasses } from "@/spx/api/ingest/event-schema";
import { mappingInputCounts } from "@/spx/api/ingest/inputs";
import { deleteMapping, getMapping, listMappingPage, mappingKey } from "@/spx/api/ingest/mappings";
import type { ApiMappingDetail, ApiMappingSummary } from "@/spx/api/ingest/schemas";

import { useCan } from "../components/access";
import { CollectionBoundary, CollectionState } from "../components/collection-state";
import { IngestConfirm } from "../components/ingest-feedback";
import { ListPagination, ListRecordLink, ListSelect } from "../components/list-controls";
import { SearchField } from "../components/search-field";
import { ListSurface, Toolbar } from "../components/toolbar";
import { useListLocation } from "../components/use-list-location";
import { MixedRuleEditor } from "./mixed-rule-editor";
import { parserModes } from "./rule-catalog";
import { RuleEventTypePicker } from "./rule-event-type-picker";

export function MappingsPage({ id }: { id?: string }) {
  const router = useRouter();
  const format = useFormatter();
  const t = useTranslations("Mappings");
  const locale = useLocale();
  const shared = useTranslations("Ingest");
  const canWrite = useCan(permissions.IngestParsingRuleWrite);
  const client = useQueryClient();
  const canReadInputs = useCan(permissions.IngestInputRead);
  const { params, update, pageControls, limit, offset } = useListLocation(20);
  const query = params.get("q") ?? "";
  const selectedOrigin = params.get("origin");
  const origin = selectedOrigin === "builtin" || selectedOrigin === "custom" ? selectedOrigin : "all";
  const mode = parserModes.find((value) => value === params.get("parse_type"));
  const rawClass = params.get("class_uid") ?? "";
  const classUID = /^\d{1,9}$/.test(rawClass) ? Number(rawClass) : undefined;
  const filters = {
    limit,
    offset,
    q: query.slice(0, 255),
    origin: origin === "all" ? undefined : origin,
    parse_type: mode,
    class_uid: classUID,
  };
  const catalog = useQuery({
    queryKey: ["ingest", "event-schema"],
    queryFn: ({ signal }) => listEventClasses(signal),
    enabled: !id,
  });
  const classLabels = new Map(catalog.data?.classes.map((item) => [item.class_uid, item.name[locale]]));
  const mappings = useQuery({
    queryKey: [...mappingKey, filters],
    queryFn: ({ signal }) => listMappingPage(filters, signal),
    enabled: !id,
  });
  const rows = mappings.data?.items;
  const ids = (rows ?? []).map((item) => item.id).sort();
  const inputCounts = useQuery({
    queryKey: ["ingest", "inputs", "counts-by-rule", ids],
    queryFn: ({ signal }) => mappingInputCounts(ids, signal),
    enabled: canReadInputs && ids.length > 0 && !id,
  });
  const detail = useQuery({
    queryKey: [...mappingKey, "detail", id],
    queryFn: ({ signal }) => getMapping(id ?? "", signal),
    enabled: !!id && id !== "new",
  });
  const hasFilters = Boolean(query || mode || classUID !== undefined || origin !== "all");
  const clear = () => window.history.replaceState(null, "", "/ingest/parsing-rules");
  function mappingHref(ruleID?: string, mode?: "edit" | "copy") {
    const search = new URLSearchParams(params.toString());
    search.delete("edit");
    search.delete("copy");
    if (mode) search.set(mode, "1");
    return `/ingest/parsing-rules${ruleID ? `/${ruleID}` : ""}${search.size ? `?${search}` : ""}`;
  }
  const editing = id === "new" ? "new" : detail.data;
  const copyRequested = canWrite && editing !== "new" && params.get("copy") === "1";
  const editingExisting = canWrite && !detail.data?.builtin && params.get("edit") === "1";
  const readOnly = id !== "new" && !copyRequested && !editingExisting;
  const [deleting, setDeleting] = useState<ApiMappingSummary>();
  const [blocked, setBlocked] = useState<ApiMappingSummary>();
  const boundInputCount = deleting ? inputCounts.data?.[deleting.id] : undefined;
  const remove = useMutation({
    mutationFn: deleteMapping,
    onSuccess: async () => {
      setDeleting(undefined);
      await client.invalidateQueries({ queryKey: mappingKey });
    },
    onError: (error, id) => {
      if (error instanceof APIError && error.status === 409 && deleting?.id === id) {
        setBlocked(deleting);
        setDeleting(undefined);
        if (canReadInputs) void inputCounts.refetch();
      }
    },
  });

  if (id && detail.error instanceof APIError && detail.error.status === 404) notFound();
  if (id) {
    const editor = {
      copyRequested,
      readOnly,
      mapping: editing === "new" ? undefined : editing,
      canWrite,
      onClose: () => router.push(mappingHref()),
      onSave: (saved: ApiMappingDetail) => {
        if (saved.id === id) router.push(mappingHref());
        else router.push("/ingest/parsing-rules?" + new URLSearchParams({ q: saved.name }).toString());
        void client.invalidateQueries({ queryKey: mappingKey });
      },
    };
    return (
      <CollectionBoundary query={detail} pageTitle={t("mappings")} hasContent={!!editing} title={t("loadFailed")}>
        {editing && (
          <MixedRuleEditor
            key={`${id}:${copyRequested}:${readOnly}`}
            {...editor}
            onCopy={() => router.push(mappingHref(id, "copy"))}
            onEdit={() => router.push(mappingHref(id, "edit"))}
          />
        )}
      </CollectionBoundary>
    );
  }
  return (
    <div className="min-w-0">
      <ListSurface>
        <Toolbar>
          <SearchField placeholder={t("search")} value={query} onChange={(v) => update("q", v)} />
          <ListSelect
            label={t("origin")}
            value={origin}
            onChange={(v) => update("origin", v === "all" ? "" : v)}
            options={[
              { value: "all", label: t("allOrigins") },
              { value: "builtin", label: t("builtin") },
              { value: "custom", label: t("custom") },
            ]}
          />
          <ListSelect
            label={t("parserType")}
            value={mode || "all"}
            options={[
              { value: "all", label: t("allParserTypes") },
              ...parserModes.map((value) => ({ value, label: t(value) })),
            ]}
            onChange={(v) => update("parse_type", v === "all" ? "" : v)}
          />
          <RuleEventTypePicker
            filterMode
            items={catalog.data?.classes ?? []}
            value={classUID === undefined ? "" : String(classUID)}
            onChange={(v) => update("class_uid", v)}
          />
          {hasFilters && (
            <Button size="sm" variant="ghost" onClick={clear}>
              {shared("clear")}
            </Button>
          )}
        </Toolbar>
        {blocked && (
          <Alert variant="destructive">
            <AlertDescription>{t("inUse", { name: blocked.name })}</AlertDescription>
          </Alert>
        )}
        <CollectionBoundary query={mappings} hasContent={!!rows?.length} title={t("loadFailed")}>
          {rows && (
            <div className="min-w-0 border-t">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("name")}</TableHead>
                    <TableHead className="hidden @min-[47rem]/list:table-cell">{t("vendor")}</TableHead>
                    <TableHead className="hidden @min-[47rem]/list:table-cell">{t("product")}</TableHead>
                    <TableHead className="hidden @min-[31rem]/list:table-cell">{t("parserType")}</TableHead>
                    <TableHead className="hidden @min-[41rem]/list:table-cell">{t("eventType")}</TableHead>
                    <TableHead>{t("origin")}</TableHead>
                    {canReadInputs && <TableHead>{t("dataInputs")}</TableHead>}
                    <TableHead className="hidden @min-[43rem]/list:table-cell">{shared("updated")}</TableHead>
                    <TableHead>
                      <span className="sr-only">{shared("actions")}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={canReadInputs ? 9 : 8} className="p-0">
                        {hasFilters ? (
                          <CollectionState kind="filtered" onClear={clear} />
                        ) : (
                          <CollectionState kind="empty" title={t("empty")} />
                        )}
                      </TableCell>
                    </TableRow>
                  )}
                  {rows.map((mapping) => {
                    const parserType = parserModes.find((value) => value === mapping.parse_type);
                    const count = inputCounts.data?.[mapping.id] ?? 0;
                    const inputCountFallback = inputCounts.data ? format.number(0) : "—";
                    return (
                      <TableRow key={mapping.id}>
                        <TableCell>
                          <ListRecordLink href={mappingHref(mapping.id)} name={mapping.name} />
                          {mapping.description && (
                            <p
                              title={mapping.description}
                              className="text-muted-foreground hidden max-w-[28cqi] truncate pt-0.5 text-xs @min-[31rem]/list:block"
                            >
                              {mapping.description}
                            </p>
                          )}
                        </TableCell>
                        <TableCell className="hidden @min-[47rem]/list:table-cell">{mapping.vendor || "—"}</TableCell>
                        <TableCell className="hidden @min-[47rem]/list:table-cell">{mapping.product || "—"}</TableCell>
                        <TableCell className="hidden @min-[31rem]/list:table-cell">
                          {parserType ? t(parserType) : "—"}
                        </TableCell>
                        <TableCell className="hidden max-w-40 whitespace-normal @min-[41rem]/list:table-cell">
                          {mapping.class_uids
                            ?.map((uid) => classLabels.get(uid))
                            .filter(Boolean)
                            .join(" · ") || "—"}
                        </TableCell>
                        <TableCell>
                          <Badge variant={mapping.builtin ? "secondary" : "outline"}>
                            {mapping.builtin ? t("builtin") : t("custom")}
                          </Badge>
                        </TableCell>
                        {canReadInputs && (
                          <TableCell>
                            {count > 0 ? (
                              <Button variant="link" className="h-auto p-0" asChild>
                                <Link
                                  aria-label={t("viewDataInputs", { name: mapping.name, count })}
                                  href={`/ingest/inputs?rule=${encodeURIComponent(mapping.id)}`}
                                >
                                  {format.number(count)}
                                </Link>
                              </Button>
                            ) : (
                              inputCountFallback
                            )}
                          </TableCell>
                        )}
                        <TableCell className="hidden @min-[43rem]/list:table-cell">
                          {format.dateTime(new Date(mapping.updated_at), { dateStyle: "medium", timeStyle: "short" })}
                        </TableCell>
                        <TableCell>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button
                                size="icon-sm"
                                variant="ghost"
                                aria-label={shared("rowActions", { name: mapping.name })}
                              >
                                <MoreHorizontalIcon />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuGroup>
                                <DropdownMenuItem asChild>
                                  <Link href={mappingHref(mapping.id)}>{t("view")}</Link>
                                </DropdownMenuItem>
                                {canWrite && !mapping.builtin && (
                                  <DropdownMenuItem asChild>
                                    <Link href={mappingHref(mapping.id, "edit")}>{t("edit")}</Link>
                                  </DropdownMenuItem>
                                )}
                                {canWrite && (
                                  <DropdownMenuItem asChild>
                                    <Link href={mappingHref(mapping.id, "copy")}>{t("copyRule")}</Link>
                                  </DropdownMenuItem>
                                )}
                                {canWrite && !mapping.builtin && (
                                  <DropdownMenuItem
                                    variant="destructive"
                                    onSelect={() => {
                                      remove.reset();
                                      setBlocked(undefined);
                                      setDeleting(mapping);
                                    }}
                                  >
                                    {t("delete")}
                                  </DropdownMenuItem>
                                )}
                              </DropdownMenuGroup>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
              <ListPagination {...pageControls(mappings.data)} />
            </div>
          )}
        </CollectionBoundary>
      </ListSurface>
      {deleting && (
        <IngestConfirm
          title={t("delete")}
          description={
            boundInputCount ? t("inUse", { name: deleting.name }) : t("deleteWarning", { name: deleting.name })
          }
          action={t("delete")}
          pending={remove.isPending}
          error={remove.isError}
          destructive
          disabled={!!boundInputCount}
          onCancel={() => setDeleting(undefined)}
          onConfirm={() => remove.mutate(deleting.id)}
        />
      )}
    </div>
  );
}
