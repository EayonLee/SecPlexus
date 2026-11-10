"use client";

import { useState } from "react";

import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, SearchIcon } from "lucide-react";
import { useFormatter, useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/native-select";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { listAuditEvents } from "@/spx/api/iam/management";
import type { AuditQuery } from "@/spx/api/iam/schemas";

import { PageHeader } from "../components/page-header";
import { StatusBadge } from "../components/status-badge";
import { Toolbar } from "../components/toolbar";
import { sessionScope } from "../iam/query";
import { errorMessageKey, useIdentity } from "../iam/session";

// AuditPage 是唯一的审计列表。可见范围由服务端按权限裁剪，
// 这里的筛选条件只收窄结果：操作者、时间范围、结果。
// 分页只向前：游标由服务端签发，跳页没有意义，也避免深 OFFSET 扫描历史。
export function AuditPage() {
  const t = useTranslations("Management");
  const common = useTranslations("Common");
  const errors = useTranslations("Errors");
  const navigation = useTranslations("Navigation");
  const format = useFormatter();
  const auth = useIdentity();

  const [draft, setDraft] = useState<AuditQuery>({});
  const [filter, setFilter] = useState<AuditQuery>({});
  const [pageSize, setPageSize] = useState(20);
  // cursors 保存已经走过的游标；index 指向当前页，回退不需要重新请求历史。
  const [cursors, setCursors] = useState<string[]>([""]);
  const [index, setIndex] = useState(0);

  const query = useQuery({
    queryKey: ["audit", ...sessionScope(auth), filter, cursors[index], pageSize],
    enabled: auth.status === "authenticated",
    queryFn: ({ signal }) =>
      listAuditEvents({ ...filter, limit: pageSize, cursor: cursors[index] || undefined }, signal),
  });

  const reset = (next: AuditQuery) => {
    setFilter(next);
    setCursors([""]);
    setIndex(0);
  };

  const items = query.data?.items ?? [];
  const nextCursor = query.data?.next_cursor ?? "";
  const loading = query.isFetching;
  const body = items.length ? (
    items.map((row) => (
      <TableRow key={row.id}>
        <TableCell>
          {format.dateTime(new Date(row.occurred_at), { dateStyle: "medium", timeStyle: "medium" })}
        </TableCell>
        <TableCell>{row.actor_name || t("system")}</TableCell>
        <TableCell>{row.action}</TableCell>
        <TableCell>
          <StatusBadge status={row.outcome}>
            {({ success: t("success"), failure: t("failure"), denied: t("denied") } as Record<string, string>)[
              row.outcome
            ] ?? row.outcome}
          </StatusBadge>
        </TableCell>
        <TableCell>{row.request_id}</TableCell>
      </TableRow>
    ))
  ) : (
    <TableRow>
      <TableCell colSpan={5}>
        <Empty>
          <EmptyHeader>
            <EmptyTitle>{common("empty")}</EmptyTitle>
            <EmptyDescription>{common("emptyDescription")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      </TableCell>
    </TableRow>
  );

  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={navigation("audit")} />

      <Toolbar
        actions={
          <>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setDraft({});
                reset({});
              }}
            >
              {t("auditReset")}
            </Button>
            <Button size="sm" onClick={() => reset(draft)}>
              {t("search")}
            </Button>
          </>
        }
      >
        <div className="flex flex-wrap items-center gap-2">
          <InputGroup className="w-56">
            <InputGroupAddon>
              <SearchIcon />
            </InputGroupAddon>
            <InputGroupInput
              id="audit-actor"
              className="h-8"
              placeholder={t("auditActorPlaceholder")}
              aria-label={t("actor")}
              value={draft.actor ?? ""}
              onChange={(event) => setDraft((current) => ({ ...current, actor: event.target.value || undefined }))}
            />
          </InputGroup>
          <InputGroup className="w-48">
            <InputGroupAddon>{t("auditFrom")}</InputGroupAddon>
            <InputGroupInput
              id="audit-from"
              type="date"
              className="h-8"
              aria-label={t("auditFrom")}
              value={draft.from ?? ""}
              onChange={(event) => setDraft((current) => ({ ...current, from: event.target.value || undefined }))}
            />
          </InputGroup>
          <InputGroup className="w-48">
            <InputGroupAddon>{t("auditTo")}</InputGroupAddon>
            <InputGroupInput
              id="audit-to"
              type="date"
              className="h-8"
              aria-label={t("auditTo")}
              value={draft.to ?? ""}
              onChange={(event) => setDraft((current) => ({ ...current, to: event.target.value || undefined }))}
            />
          </InputGroup>
          <NativeSelect
            id="audit-outcome"
            className="h-8 w-32"
            aria-label={t("outcome")}
            value={draft.outcome ?? ""}
            onChange={(event) =>
              setDraft((current) => ({
                ...current,
                outcome: (event.target.value || undefined) as AuditQuery["outcome"],
              }))
            }
          >
            <option value="">{t("auditOutcomeAny")}</option>
            <option value="success">{t("success")}</option>
            <option value="failure">{t("failure")}</option>
            <option value="denied">{t("denied")}</option>
          </NativeSelect>
        </div>
      </Toolbar>

      <div className="flex min-w-0 flex-col gap-4">
        {query.error && <p role="alert">{errors(errorMessageKey(query.error))}</p>}
        <div className="overflow-hidden rounded-xl border">
          <Table>
            <TableHeader className="bg-muted">
              <TableRow>
                <TableHead>{t("time")}</TableHead>
                <TableHead>{t("actor")}</TableHead>
                <TableHead>{t("action")}</TableHead>
                <TableHead>{t("outcome")}</TableHead>
                <TableHead>{t("requestId")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={5}>
                    <Spinner />
                  </TableCell>
                </TableRow>
              ) : (
                body
              )}
            </TableBody>
          </Table>
        </div>
        <div className="flex items-center justify-between gap-6">
          <p className="text-muted-foreground text-sm whitespace-nowrap">{t("auditPage", { page: index + 1 })}</p>
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2">
              <Label className="whitespace-nowrap">{common("rowsPerPage")}</Label>
              <Select
                value={String(pageSize)}
                onValueChange={(value) => {
                  setPageSize(Number(value));
                  setCursors([""]);
                  setIndex(0);
                }}
              >
                <SelectTrigger size="sm" className="w-20" aria-label={common("rowsPerPage")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent side="top">
                  <SelectGroup>
                    {[10, 20, 50].map((size) => (
                      <SelectItem key={size} value={String(size)}>
                        {size}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="icon-sm"
                aria-label={t("auditNewer")}
                disabled={index === 0 || loading}
                onClick={() => setIndex(index - 1)}
              >
                <ChevronLeft />
              </Button>
              <Button
                variant="outline"
                size="icon-sm"
                aria-label={t("auditOlder")}
                disabled={!nextCursor || loading}
                onClick={() => {
                  setCursors((current) => [...current.slice(0, index + 1), nextCursor]);
                  setIndex(index + 1);
                }}
              >
                <ChevronRight />
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
