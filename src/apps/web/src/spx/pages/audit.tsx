"use client";

import { useState } from "react";

import { useQuery } from "@tanstack/react-query";
import { useFormatter, useTimeZone, useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { listAuditEvents } from "@/spx/api/iam/management";
import { auditDateBounds } from "@/spx/api/iam/audit-dates";
import type { AuditQuery } from "@/spx/api/iam/schemas";

import { CollectionBoundary, CollectionState } from "../components/collection-state";
import { ListDateFilter } from "../components/list-date-filter";
import { ListPagination, ListSelect } from "../components/list-controls";
import { PageHeader } from "../components/page-header";
import { SearchField } from "../components/search-field";
import { StatusBadge } from "../components/status-badge";
import { ListSurface, Toolbar } from "../components/toolbar";
import { sessionScope } from "../iam/query";
import { useIdentity } from "../iam/session";

// AuditPage 是唯一的审计列表。可见范围由服务端按权限裁剪，
// 这里的筛选条件只收窄结果：操作者、时间范围、结果。
// 分页只向前：游标由服务端签发，跳页没有意义，也避免深 OFFSET 扫描历史。
export function AuditPage() {
  const t = useTranslations("Management");
  const navigation = useTranslations("Navigation");
  const format = useFormatter();
  const timeZone = useTimeZone() ?? "UTC";
  const auth = useIdentity();

  const [draft, setDraft] = useState<AuditQuery>({});
  const [filter, setFilter] = useState<AuditQuery>({});
  const [pageSize, setPageSize] = useState(20);
  // cursors 保存已经走过的游标；index 指向当前页，回退不需要重新请求历史。
  const [cursors, setCursors] = useState<string[]>([""]);
  const [index, setIndex] = useState(0);

  const query = useQuery({
    queryKey: ["audit", ...sessionScope(auth), filter, timeZone, cursors[index], pageSize],
    enabled: auth.status === "authenticated",
    queryFn: ({ signal }) =>
      listAuditEvents({
        ...filter, ...auditDateBounds(filter.from, filter.to, timeZone),
        limit: pageSize, cursor: cursors[index] || undefined,
      }, signal),
  });

  const reset = (next: AuditQuery) => {
    setFilter(next);
    setCursors([""]);
    setIndex(0);
  };

  const clearFilters = () => {
    setDraft({});
    reset({});
  };
  const items = query.data?.items ?? [];
  const nextCursor = query.data?.next_cursor ?? "";
  const loading = query.isFetching;
  const body = items.length ? (
    items.map((row) => (
      <TableRow key={row.id} className="border-border/60 hover:bg-muted/20">
        <TableCell className="whitespace-normal">
          <div>{format.dateTime(new Date(row.occurred_at), { dateStyle: "medium", timeStyle: "medium" })}</div>
          <span className="text-muted-foreground block text-xs [overflow-wrap:anywhere] @lg/list:hidden">
            {row.actor_name || t("system")}
          </span>
        </TableCell>
        <TableCell className="hidden @lg/list:table-cell">{row.actor_name || t("system")}</TableCell>
        <TableCell className="whitespace-normal [overflow-wrap:anywhere]">
          {row.action}
          <span className="text-muted-foreground block text-xs @lg/list:hidden">{row.request_id}</span>
        </TableCell>
        <TableCell>
          <StatusBadge status={row.outcome}>
            {({ success: t("success"), failure: t("failure"), denied: t("denied") } as Record<string, string>)[
              row.outcome
            ] ?? row.outcome}
          </StatusBadge>
        </TableCell>
        <TableCell className="hidden @lg/list:table-cell">{row.request_id}</TableCell>
      </TableRow>
    ))
  ) : (
    <TableRow>
      <TableCell colSpan={5} className="p-0">
        {Object.values(filter).some(Boolean) ? (
          <CollectionState kind="filtered" onClear={clearFilters} />
        ) : (
          <CollectionState kind="empty" />
        )}
      </TableCell>
    </TableRow>
  );

  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={navigation("audit")} />

      <ListSurface>
        <Toolbar
          actions={
            <>
              <Button size="sm" variant="outline" onClick={clearFilters}>
                {t("auditReset")}
              </Button>
              <Button size="sm" onClick={() => reset(draft)}>
                {t("search")}
              </Button>
            </>
          }
        >
          <div className="flex w-full flex-wrap items-center gap-2">
            <SearchField
              value={draft.actor ?? ""}
              placeholder={t("auditActorPlaceholder")}
              onChange={(actor) => setDraft((current) => ({ ...current, actor: actor || undefined }))}
              onSubmit={(actor) => reset({ ...draft, actor: actor || undefined })}
            />
            <ListDateFilter
              label={t("auditFrom")}
              value={draft.from}
              max={draft.to}
              onChange={(from) => setDraft((current) => ({ ...current, from }))}
            />
            <ListDateFilter
              label={t("auditTo")}
              value={draft.to}
              min={draft.from}
              onChange={(to) => setDraft((current) => ({ ...current, to }))}
            />
            <ListSelect<NonNullable<AuditQuery["outcome"]> | "all">
              label={t("outcome")}
              value={draft.outcome ?? "all"}
              options={[
                { value: "all", label: t("auditOutcomeAny") },
                ...(["success", "failure", "denied"] as const).map((value) => ({ value, label: t(value) })),
              ]}
              onChange={(outcome) =>
                setDraft((current) => ({ ...current, outcome: outcome === "all" ? undefined : outcome }))
              }
            />
          </div>
        </Toolbar>

        <div className="flex min-w-0 flex-col">
          <CollectionBoundary query={query} hasContent={items.length > 0}>
            <div className="border-y">
              <Table className="**:data-[slot=table-cell]:px-2 **:data-[slot=table-head]:px-2 [&_td]:py-3 @lg/list:**:data-[slot=table-cell]:px-4 @lg/list:**:data-[slot=table-head]:px-4">
                <TableHeader className="[&_th]:text-muted-foreground [&_th]:h-11">
                  <TableRow>
                    <TableHead className="w-[32%] @lg/list:w-auto">{t("time")}</TableHead>
                    <TableHead className="hidden @lg/list:table-cell">{t("actor")}</TableHead>
                    <TableHead>{t("action")}</TableHead>
                    <TableHead className="w-0">{t("outcome")}</TableHead>
                    <TableHead className="hidden @lg/list:table-cell">{t("requestId")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>{body}</TableBody>
              </Table>
            </div>
            <ListPagination
              index={index}
              size={pageSize}
              hasNext={Boolean(nextCursor)}
              loading={loading}
              onPage={(next) => {
                if (next > index) setCursors((current) => [...current.slice(0, index + 1), nextCursor]);
                setIndex(next);
              }}
              onSize={(size) => {
                setPageSize(size);
                setCursors([""]);
                setIndex(0);
              }}
            />
          </CollectionBoundary>
        </div>
      </ListSurface>
    </div>
  );
}
