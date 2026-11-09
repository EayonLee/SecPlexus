"use client";

import { useState } from "react";

import { useFormatter, useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import { listAuditEvents } from "@/spx/api/iam/management";
import type { ApiAuditEvent, AuditQuery } from "@/spx/api/iam/schemas";

import { DataTable, usePagedQuery } from "../components/data";
import { StatusBadge } from "../components/status-badge";

// AuditPage 是唯一的审计列表。可见范围由服务端按权限裁剪，
// 这里的筛选条件只收窄结果：操作者、时间范围、结果。
export function AuditPage() {
  const t = useTranslations("Management");
  const format = useFormatter();

  const [draft, setDraft] = useState<AuditQuery>({});
  const [applied, setApplied] = useState<AuditQuery>({});

  const state = usePagedQuery<ApiAuditEvent>(["audit", applied], (query, signal) =>
    listAuditEvents({ ...applied, ...query }, signal),
  );

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-base font-medium">{t("auditTitle")}</h2>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-wrap items-end gap-2">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="audit-actor" className="text-muted-foreground text-xs font-medium">
              {t("actor")}
            </label>
            <Input
              id="audit-actor"
              className="h-9 w-44"
              value={draft.actor ?? ""}
              placeholder={t("auditActorPlaceholder")}
              onChange={(event) => setDraft((current) => ({ ...current, actor: event.target.value || undefined }))}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="audit-from" className="text-muted-foreground text-xs font-medium">
              {t("auditFrom")}
            </label>
            <Input
              id="audit-from"
              type="date"
              className="h-9 w-40"
              value={draft.from ?? ""}
              onChange={(event) => setDraft((current) => ({ ...current, from: event.target.value || undefined }))}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="audit-to" className="text-muted-foreground text-xs font-medium">
              {t("auditTo")}
            </label>
            <Input
              id="audit-to"
              type="date"
              className="h-9 w-40"
              value={draft.to ?? ""}
              onChange={(event) => setDraft((current) => ({ ...current, to: event.target.value || undefined }))}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="audit-outcome" className="text-muted-foreground text-xs font-medium">
              {t("outcome")}
            </label>
            <NativeSelect
              id="audit-outcome"
              className="h-9 w-32"
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
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => { setDraft({}); setApplied({}); }}>
            {t("auditReset")}
          </Button>
          <Button onClick={() => setApplied(draft)}>{t("auditSearch")}</Button>
        </div>
      </div>

      <DataTable
        state={state}
        columns={[
          {
            title: t("time"),
            cell: (row) => format.dateTime(new Date(row.occurred_at), { dateStyle: "medium", timeStyle: "medium" }),
          },
          { title: t("actor"), cell: (row) => row.actor_name || t("system") },
          { title: t("action"), cell: (row) => row.action },
          {
            title: t("outcome"),
            cell: (row) => (
              <StatusBadge status={row.outcome}>
                {({ success: t("success"), failure: t("failure"), denied: t("denied") } as Record<string, string>)[
                  row.outcome
                ] ?? row.outcome}
              </StatusBadge>
            ),
          },
          { title: t("requestId"), cell: (row) => row.request_id },
        ]}
      />
    </div>
  );
}
