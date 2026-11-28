"use client";

import { useState } from "react";

import Link from "next/link";
import { notFound, useRouter } from "next/navigation";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, CopyIcon, MoreHorizontalIcon } from "lucide-react";
import { useFormatter, useTranslations } from "next-intl";

import { permissions } from "@/auth/permissions.gen";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { APIError } from "@/spx/api/client";
import { deleteInput, getInput, listInputPage, updateInput } from "@/spx/api/ingest/inputs";
import { listMappingPage, mappingKey } from "@/spx/api/ingest/mappings";
import type { ApiInput, UpdateInputBody } from "@/spx/api/ingest/schemas";

import { useCan } from "../components/access";
import { CollectionBoundary, CollectionState } from "../components/collection-state";
import { IngestConfirm } from "../components/ingest-feedback";
import { ListPagination, ListRecordLink, ListSelect } from "../components/list-controls";
import { SearchField } from "../components/search-field";
import { SheetPanel } from "../components/sheet-panel";
import { Toolbar } from "../components/toolbar";
import { useListLocation } from "../components/use-list-location";
import { channelIcons, protocols, RulePicker, useMappingDetail } from "./ingest-input-controls";
import { InputDetails } from "./ingest-input-details";
import { InputWizard } from "./ingest-input-wizard";

const inputKey = ["ingest", "inputs"] as const;

export function IngestPage({ id }: { id?: string }) {
  const router = useRouter();
  const format = useFormatter();
  const { params, update, pageControls, limit, offset } = useListLocation(20);
  const query = params.get("q") ?? "";
  const protocolFilter = protocols.find((item) => item === params.get("protocol")) ?? "all";
  const status = params.get("status");
  const statusFilter = status === "enabled" || status === "disabled" ? status : "all";
  const canVerify = useCan(permissions.IngestParsingRuleWrite);
  const t = useTranslations("Ingest");
  const navigation = useTranslations("Navigation");
  const mappingText = useTranslations("Mappings");
  const canWrite = useCan(permissions.IngestInputWrite);
  const client = useQueryClient();
  const canReadRules = useCan(permissions.IngestParsingRuleRead);
  const [created, setCreated] = useState<ApiInput>();
  const resultPage = id === "new" && !!created;
  const wizard = id === "new" && !created;
  const details = id === "new" ? created?.id : id;
  function inputHref(value?: string, edit = false) {
    const search = new URLSearchParams(params.toString());
    search.delete("via");
    if (edit) search.set("edit", "1");
    else search.delete("edit");
    return `/ingest/inputs${value ? `/${value}` : ""}${search.size ? `?${search}` : ""}`;
  }
  const setDetails = (value?: string) => router.push(inputHref(value));
  const rule = params.get("rule") || undefined;
  const boundaryRule = useMappingDetail(rule);
  const filters = {
    limit,
    offset,
    q: query.slice(0, 255),
    protocol: protocolFilter === "all" ? undefined : protocolFilter,
    status: statusFilter === "all" ? undefined : statusFilter,
    rule,
  };
  const inputs = useQuery({
    queryKey: [...inputKey, filters],
    queryFn: ({ signal }) => listInputPage(filters, signal),
    enabled: !id,
  });
  const detail = useQuery({
    queryKey: [...inputKey, "detail", details],
    queryFn: ({ signal }) => getInput(details ?? "", signal),
    refetchInterval: 5000,
    enabled: !!details,
  });
  const [confirmation, setConfirmation] = useState<{ input: ApiInput; operation: "delete" | "rotate" | "pause" }>();
  const [issued, setIssued] = useState<{ input: ApiInput; token: string }>();
  const [copied, setCopied] = useState(false);
  const current = detail.data ?? (resultPage && detail.isPending ? created : undefined);
  const rows = inputs.data?.items;
  const ruleIDs = [...new Set((rows ?? []).flatMap((row) => (row.parsing_rule_id ? [row.parsing_rule_id] : [])))].sort();
  const pageRules = useQuery({
    queryKey: [...mappingKey, "input-rows", ruleIDs],
    queryFn: ({ signal }) => listMappingPage({ limit: ruleIDs.length, id: ruleIDs }, signal),
    enabled: canReadRules && ruleIDs.length > 0 && !id,
  });
  const hasFilters = Boolean(rule || query || protocolFilter !== "all" || statusFilter !== "all");
  const refresh = () => client.invalidateQueries({ queryKey: inputKey });
  const [selection, setSelection] = useState<string[]>([]);
  const [batch, setBatch] = useState<{ enabled: boolean; items: ApiInput[] }>();
  const selected = (rows ?? []).filter((input) => selection.includes(input.id));
  const [failedNames, setFailedNames] = useState<string[]>([]);
  const bulk = useMutation({
    mutationFn: async ({ items, enabled }: { items: ApiInput[]; enabled: boolean }) => {
      const failed: ApiInput[] = [];
      for (const item of items) {
        try {
          await updateInput(item.id, { enabled });
        } catch {
          failed.push(item);
        }
      }
      return failed;
    },
    onSuccess: async (failed) => {
      setSelection(failed.map((item) => item.id));
      setFailedNames(failed.map((item) => item.name));
      setBatch(undefined);
      await refresh();
    },
  });
  function selectBatch(enabled: boolean) {
    setFailedNames([]);
    setBatch({ enabled, items: selected.filter((item) => item.enabled !== enabled) });
  }
  const change = useMutation({
    mutationFn: async ({ id, body }: { id: string; body: UpdateInputBody }) => {
      const result = await updateInput(id, body);
      if (result.token) {
        setCopied(false);
        setIssued({ input: result.input, token: result.token });
      }
      setConfirmation(undefined);
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: deleteInput,
    onSuccess: async () => {
      setConfirmation(undefined);
      setDetails(undefined);
      await refresh();
    },
  });

  function dismissToken(inputID: string) {
    if (wizard) router.replace(`/ingest/inputs/${inputID}`);
    setIssued(undefined);
  }
  if (details && detail.error instanceof APIError && detail.error.status === 404) notFound();
  function toggleInput(input: ApiInput) {
    change.reset();
    if (input.enabled) setConfirmation({ input, operation: "pause" });
    else change.mutate({ id: input.id, body: { enabled: true } });
  }
  function inputActions(input: ApiInput) {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="icon-sm" variant="ghost" aria-label={t("rowActions", { name: input.name })}>
            <MoreHorizontalIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuGroup>
            {!details && (
              <DropdownMenuItem asChild>
                <Link href={inputHref(input.id, true)}>{t("editConfig")}</Link>
              </DropdownMenuItem>
            )}
            <DropdownMenuItem className={!details ? "xl:hidden" : undefined} onSelect={() => toggleInput(input)}>
              {input.enabled ? t("pause") : t("resume")}
            </DropdownMenuItem>
            {input.protocol === "http" && (
              <DropdownMenuItem
                onSelect={() => {
                  change.reset();
                  setConfirmation({ input, operation: "rotate" });
                }}
              >
                {t("rotate")}
              </DropdownMenuItem>
            )}
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => {
                remove.reset();
                setConfirmation({ input, operation: "delete" });
              }}
            >
              {t("delete")}
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }
  return (
    <div className="flex min-w-0 flex-col">
      {!id && (
        <Toolbar>
          <SearchField value={query} onChange={(v) => update("q", v)} placeholder={t("search")} />
          <ListSelect
            label={t("protocol")}
            value={protocolFilter}
            options={[
              { value: "all", label: t("allProtocols") },
              ...protocols.map((value) => ({
                value,
                label: t(value),
              })),
            ]}
            onChange={(v) => update("protocol", v === "all" ? "" : v)}
          />
          <ListSelect
            label={t("status")}
            value={statusFilter}
            options={[
              { value: "all", label: t("allStatuses") },
              ...(["enabled", "disabled"] as const).map((value) => ({
                value,
                label: t(value),
              })),
            ]}
            onChange={(v) => update("status", v === "all" ? "" : v)}
          />
          {canReadRules && (
            <RulePicker
              filterMode
              value={rule ?? ""}
              selected={boundaryRule.data}
              onChange={(id) => update("rule", id)}
            />
          )}
          {hasFilters && !!rows?.length && (
            <Button variant="ghost" onClick={() => window.history.replaceState(null, "", "/ingest/inputs")}>
              {t("clear")}
            </Button>
          )}
        </Toolbar>
      )}
      {!id && canWrite && selected.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 border-t bg-muted/30 px-4 py-2 text-sm">
          <span>{t("selected", { count: selected.length })}</span>
          <Button
            size="sm"
            variant="outline"
            disabled={!selected.some((item) => !item.enabled)}
            onClick={() => selectBatch(true)}
          >
            {t("resumeShort")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={!selected.some((item) => item.enabled)}
            onClick={() => selectBatch(false)}
          >
            {t("pauseShort")}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setSelection([])}>
            {t("clearSelection")}
          </Button>
        </div>
      )}
      {!id && failedNames.length > 0 && (
        <Alert variant="destructive">
          <AlertDescription>{t("batchFailed", { names: format.list(failedNames) })}</AlertDescription>
        </Alert>
      )}
      {batch && (
        <IngestConfirm
          title={t(batch.enabled ? "resumeShort" : "pauseShort")}
          description={t(batch.enabled ? "batchResume" : "batchPause", { count: batch.items.length })}
          action={t(batch.enabled ? "resumeShort" : "pauseShort")}
          destructive={!batch.enabled}
          pending={bulk.isPending}
          error={bulk.isError}
          onCancel={() => setBatch(undefined)}
          onConfirm={() => bulk.mutate(batch)}
        />
      )}
      {change.isError && !confirmation && !details && (
        <Alert variant="destructive">
          <AlertDescription>{t("operationFailed")}</AlertDescription>
        </Alert>
      )}
      {!wizard && (!details || !current) && (
        <CollectionBoundary
          query={details ? detail : inputs}
          pageTitle={details ? navigation("inputs") : undefined}
          hasContent={wizard || !!current || (!id && !!rows?.length)}
          title={t("loadFailed")}
        >
          {!id && rows && (
            <div className="min-w-0 border-t">
              <Table>
                <TableHeader>
                  <TableRow>
                    {canWrite && (
                      <TableHead className="w-9">
                        <Checkbox
                          aria-label={t("selectPage")}
                          disabled={!rows.length}
                          checked={selected.length > 0 && (selected.length === rows.length || "indeterminate")}
                          onCheckedChange={(checked) =>
                            setSelection(checked === true ? rows.map((item) => item.id) : [])
                          }
                        />
                      </TableHead>
                    )}
                    <TableHead>{t("name")}</TableHead>
                    <TableHead className="hidden @min-[60rem]/list:table-cell">{mappingText("vendor")}</TableHead>
                    <TableHead className="hidden @min-[60rem]/list:table-cell">{mappingText("product")}</TableHead>
                    <TableHead className="hidden sm:table-cell">{t("protocol")}</TableHead>
                    <TableHead className="hidden lg:table-cell">{t("mapping")}</TableHead>
                    <TableHead>{t("status")}</TableHead>
                    <TableHead className="hidden @min-[74rem]/list:table-cell">{t("received")}</TableHead>
                    <TableHead className="hidden @min-[74rem]/list:table-cell">{t("parseFailed")}</TableHead>
                    <TableHead className="hidden sm:table-cell">{t("lastReceived")}</TableHead>
                    <TableHead>
                      <span className="sr-only">{t("actions")}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={canWrite ? 11 : 10} className="p-0">
                        {hasFilters ? (
                          <CollectionState
                            kind="filtered"
                            title={t("noMatch")}
                            onClear={() => window.history.replaceState(null, "", "/ingest/inputs")}
                          />
                        ) : (
                          <CollectionState
                            kind="empty"
                            title={canWrite ? t("firstInput") : t("empty")}
                            actions={
                              canWrite &&
                              protocols.map((channel) => {
                                const Icon = channelIcons[channel];
                                return (
                                  <Button key={channel} asChild variant="outline">
                                    <Link href={`/ingest/inputs/new?via=${channel}`}>
                                      <Icon />
                                      {t(channel)}
                                      <ArrowRight />
                                    </Link>
                                  </Button>
                                );
                              })
                            }
                          />
                        )}
                      </TableCell>
                    </TableRow>
                  )}
                  {rows.map((input) => (
                    <TableRow key={input.id} data-state={selection.includes(input.id) ? "selected" : undefined}>
                      {canWrite && (
                        <TableCell>
                          <Checkbox
                            aria-label={t("selectInput", { name: input.name })}
                            checked={selection.includes(input.id)}
                            onCheckedChange={(checked) =>
                              setSelection((current) =>
                                checked === true ? [...current, input.id] : current.filter((id) => id !== input.id),
                              )
                            }
                          />
                        </TableCell>
                      )}
                      <TableCell>
                        <ListRecordLink href={inputHref(input.id)} name={input.name} />
                      </TableCell>
                      <TableCell className="hidden @min-[60rem]/list:table-cell">{input.vendor || "—"}</TableCell>
                      <TableCell className="hidden @min-[60rem]/list:table-cell">{input.product || "—"}</TableCell>
                      <TableCell className="hidden sm:table-cell">{t(input.protocol)}</TableCell>
                      <TableCell className="hidden lg:table-cell">
                        {input.parsing_rule_id && canReadRules ? (
                          <ListRecordLink
                            href={`/ingest/parsing-rules/${input.parsing_rule_id}`}
                            name={
                              pageRules.data?.items.find((mapping) => mapping.id === input.parsing_rule_id)?.name ??
                              t("bound")
                            }
                          />
                        ) : input.parsing_rule_id ? (
                          t("bound")
                        ) : (
                          t("mappingNone")
                        )}
                      </TableCell>
                      <TableCell>
                        <Badge variant={input.enabled ? "success" : "outline"}>
                          {input.enabled ? t("enabled") : t("disabled")}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden tabular-nums @min-[74rem]/list:table-cell">
                        {input.stats ? format.number(input.stats.received) : "—"}
                      </TableCell>
                      <TableCell className="hidden tabular-nums @min-[74rem]/list:table-cell">
                        {input.stats ? format.number(input.stats.parse_failed) : t("noData")}
                      </TableCell>
                      <TableCell className="hidden max-w-36 text-xs leading-5 whitespace-normal tabular-nums sm:table-cell sm:max-w-none sm:text-sm">
                        {input.stats
                          ? format.dateTime(new Date(input.stats.last_received), {
                              dateStyle: "medium",
                              timeStyle: "short",
                            })
                          : t("noData")}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <Button asChild size="sm" variant="ghost" className="hidden xl:inline-flex">
                            <Link href={inputHref(input.id)}>{mappingText("view")}</Link>
                          </Button>
                          {canWrite && (
                            <>
                              <Button
                                size="sm"
                                variant="ghost"
                                className="hidden xl:inline-flex"
                                disabled={change.isPending}
                                onClick={() => toggleInput(input)}
                              >
                                {input.enabled ? t("pauseShort") : t("resumeShort")}
                              </Button>
                              {inputActions(input)}
                            </>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <ListPagination {...pageControls(inputs.data)} />
            </div>
          )}
        </CollectionBoundary>
      )}
      {wizard && !issued && (
        <InputWizard
          canVerify={canReadRules && canVerify}
          onClose={() => setDetails()}
          onCreated={(input, token) => {
            client.setQueryData([...inputKey, "detail", input.id], input);

            if (token) {
              setCopied(false);
              setIssued({ input, token });
            }
            setCreated(input);
            window.history.replaceState(null, "", inputHref(input.id));
            void refresh();
          }}
        />
      )}
      {current && (
        <InputDetails
          key={current.id}
          refreshFailed={detail.isError}
          token={issued?.input.id === current.id && resultPage ? issued.token : undefined}
          onDismissToken={() => setIssued(undefined)}
          input={current}
          canWrite={canWrite}
          actions={canWrite && inputActions(current)}
          onRefresh={() => void detail.refetch()}
          pending={change.isPending}
          error={change.isError}
          onClose={() => {
            setDetails(undefined);
            change.reset();
          }}
          onChange={(body, onSuccess) => change.mutate({ id: current.id, body }, { onSuccess })}
        />
      )}
      {confirmation && (
        <IngestConfirm
          title={t(confirmation.operation)}
          description={t(
            ({ delete: "deleteWarning", pause: "pauseWarning", rotate: "rotateWarning" } as const)[
              confirmation.operation
            ],
            {
              name: confirmation.input.name,
            },
          )}
          action={t(confirmation.operation)}
          destructive={confirmation.operation === "delete"}
          pending={change.isPending || remove.isPending}
          error={confirmation.operation === "delete" ? remove.isError : change.isError}
          onCancel={() => setConfirmation(undefined)}
          onConfirm={() =>
            confirmation.operation === "delete"
              ? remove.mutate(confirmation.input.id)
              : change.mutate({
                  id: confirmation.input.id,
                  body: confirmation.operation === "pause" ? { enabled: false } : { rotate_token: true },
                })
          }
        />
      )}
      {issued && !resultPage && (
        <SheetPanel
          title={t("issuedToken", { name: issued.input.name })}
          onClose={() => dismissToken(issued.input.id)}
          footer={<Button onClick={() => dismissToken(issued.input.id)}>{t("done")}</Button>}
        >
          <div className="flex flex-col gap-4 px-6 py-5">
            <Alert>
              <AlertDescription>{t("tokenOnce")}</AlertDescription>
            </Alert>
            <div className="flex flex-wrap items-center gap-2">
              <code className="bg-muted rounded-md p-2 text-sm break-all">{issued.token}</code>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  void navigator.clipboard.writeText(issued.token).then(
                    () => setCopied(true),
                    () => setCopied(false),
                  );
                }}
              >
                <CopyIcon data-icon="inline-start" />
                {copied ? t("copied") : t("copy")}
              </Button>
            </div>
          </div>
        </SheetPanel>
      )}
    </div>
  );
}
