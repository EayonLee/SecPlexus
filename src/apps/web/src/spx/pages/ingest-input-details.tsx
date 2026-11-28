"use client";

import { type ReactNode, useState } from "react";

import Link from "next/link";
import { useSearchParams } from "next/navigation";

import { useFormatter, useTranslations } from "next-intl";
import { useForm, useWatch } from "react-hook-form";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CodeBlockCopyButton } from "@/components/ui/code-block";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import type { ApiInput, UpdateInputBody } from "@/spx/api/ingest/schemas";

import { useCan } from "../components/access";
import { CollectionState } from "../components/collection-state";
import { IngestSection, SourceIdentityFields } from "../components/ingest-section";
import { PageHeader } from "../components/page-header";
import { SheetPanel } from "../components/sheet-panel";
import {
  DetailFields,
  RulePicker,
  SampleTrial,
  unboundRule,
  useMappingDetail,
  useMappingTrial,
} from "./ingest-input-controls";
import { inputEndpoint } from "./ingest-model";

export function InputDetails({
  input,
  refreshFailed,
  token,
  onDismissToken,
  actions,
  onRefresh,
  canWrite,
  pending,
  error,
  onClose,
  onChange,
}: {
  input: ApiInput;
  refreshFailed: boolean;
  token?: string;
  onDismissToken: () => void;
  actions: ReactNode;
  onRefresh: () => void;
  canWrite: boolean;
  pending: boolean;
  error: boolean;
  onClose: () => void;
  onChange: (body: UpdateInputBody, onSuccess?: () => void) => void;
}) {
  const t = useTranslations("Ingest");
  const mixed = useTranslations("MixedRule");
  const format = useFormatter();
  const mappingText = useTranslations("Mappings");
  const detailParams = useSearchParams();
  const editing = canWrite && detailParams.get("edit") === "1";
  function setEditing(open: boolean) {
    const url = new URL(window.location.href);
    if (open) url.searchParams.set("edit", "1");
    else url.searchParams.delete("edit");
    window.history.replaceState(null, "", url);
  }
  const firstMessage = { http: "firstHttp", syslog: "firstSyslog", kafka: "firstKafka" } as const;
  const unlistedMapping = input.parsing_rule_id ? t("bound") : t("mappingNone");
  const { data: boundMapping } = useMappingDetail(input.parsing_rule_id ?? undefined);
  const endpoint = inputEndpoint(input);
  return (
    <section className="@container flex min-w-0 flex-col gap-4">
      <PageHeader
        title={input.name}
        actions={
          <>
            <Button onClick={onClose} variant="outline">
              {t("backToList")}
            </Button>
            {canWrite && !editing && (
              <Button size="sm" onClick={() => setEditing(true)}>
                {t("editConfig")}
              </Button>
            )}
            {actions}
          </>
        }
      />
      <Card className="min-w-0 gap-4 p-4 sm:p-6">
        {editing && (
          <InputEditSheet
            key={input.id}
            input={input}
            boundMappingName={boundMapping?.name ?? unlistedMapping}
            pending={pending}
            error={error}
            onClose={() => setEditing(false)}
            onChange={onChange}
          />
        )}
        <div className="grid min-w-0 items-start gap-8 @min-[54rem]:grid-cols-[minmax(0,1.4fr)_minmax(18rem,1fr)] @min-[54rem]:gap-12">
          <div className="grid min-w-0 gap-8">
            <IngestSection title={t("connection")}>
              <DetailFields
                rows={[
                  { label: t("protocol"), value: t(input.protocol) },
                  {
                    label: input.protocol === "kafka" ? t("topicLabel") : t("endpoint"),
                    value: endpoint ? (
                      <span className="inline-flex max-w-full items-center gap-2">
                        <code className="min-w-0 break-all">{endpoint}</code>
                        <CodeBlockCopyButton value={endpoint} />
                      </span>
                    ) : (
                      t("unconfigured")
                    ),
                  },
                  ...(input.protocol === "kafka"
                    ? [
                        { label: t("brokers"), value: input.kafka_brokers?.join(", ") || t("unconfigured") },
                        ...(input.kafka_credential
                          ? [{ label: t("kafkaCredential"), value: input.kafka_credential }]
                          : []),
                        { label: t("startPosition"), value: t(input.kafka_start_position === "since_created" ? "startSinceCreated" : "startEarliest") },
                      ]
                    : []),
                ]}
              />
              {token && (
                <section className="flex min-w-0 flex-col gap-2" aria-label={t("tokenLabel")}>
                  <p className="text-sm">{t("tokenOnce")}</p>
                  <div className="flex flex-wrap items-center gap-2">
                    <code className="break-all">{token}</code>
                    <CodeBlockCopyButton value={token} />
                    <Button variant="outline" size="sm" onClick={onDismissToken}>
                      {t("done")}
                    </Button>
                  </div>
                </section>
              )}
              {input.protocol === "http" && (
                <details className="text-sm">
                  <summary className="cursor-pointer font-medium">{t("requestExample")}</summary>
                  <pre className="mt-2 whitespace-pre-wrap break-all border-l pl-3 text-xs leading-5">
                    POST {endpoint}
                    {"\n"}Authorization: Bearer &lt;token&gt;
                  </pre>
                </details>
              )}
            </IngestSection>
            <IngestSection title={t("parseConfig")}>
              <DetailFields
                rows={[
                  { label: mappingText("vendor"), value: input.vendor || "—" },
                  { label: mappingText("product"), value: input.product || "—" },
                  {
                    label: t("mapping"),
                    value: boundMapping ? (
                      <Link className="text-primary hover:underline" href={`/ingest/parsing-rules/${boundMapping.id}`}>
                        {boundMapping.name}
                      </Link>
                    ) : (
                      unlistedMapping
                    ),
                  },
                  ...(boundMapping
                    ? [{ label: mappingText("version"), value: format.number(boundMapping.version) }]
                    : []),
                ]}
              />
            </IngestSection>
          </div>
          <div className="grid min-w-0 gap-8">
            <IngestSection
              title={t("diagnosis")}
              action={
                <Button size="sm" variant="outline" onClick={onRefresh}>
                  {t("refresh")}
                </Button>
              }
            >
              {refreshFailed && <CollectionState kind="error" retainContent onRetry={onRefresh} />}
              <DetailFields
                rows={[
                  {
                    label: t("status"),
                    value: (
                      <Badge variant={input.enabled ? "success" : "outline"}>
                        {t(input.enabled ? "enabled" : "disabled")}
                      </Badge>
                    ),
                  },
                  ...(input.activation
                    ? [{
                        label: t("activationStatus"),
                        value: input.activation.reason
                          ? t(`activationReason.${input.activation.reason}`)
                          : t(`activation.${input.activation.state}`),
                      }]
                    : []),
                  ...(input.traffic_status && input.traffic_status !== "disabled"
                    ? [{ label: t("trafficStatus"), value: t(`traffic.${input.traffic_status}`) }]
                    : []),
                  {
                    label: t("lastReceived"),
                    value: input.stats?.last_received
                      ? format.dateTime(new Date(input.stats.last_received), {
                          dateStyle: "medium",
                          timeStyle: "medium",
                        })
                      : t(input.enabled ? (input.observations_ready ? "awaitFirst" : "observationsUnknown") : "noData"),
                  },
                  {
                    label: t("lastConfirmed"),
                    value: input.stats?.last_confirmed
                      ? format.dateTime(new Date(input.stats.last_confirmed), { dateStyle: "medium", timeStyle: "medium" })
                      : t(input.enabled && !input.observations_ready ? "observationsUnknown" : "noData"),
                  },
                  ...(input.stats
                    ? [
                        {
                          label: t("received"),
                          value: (
                            <span data-testid="received-count" className="font-medium tabular-nums">
                              {format.number(input.stats.received)}
                            </span>
                          ),
                        },
                        {
                          label: t("confirmedCount"),
                          value: format.number(input.stats.confirmed ?? 0),
                        },
                        {
                          label: mixed("ok"),
                          value: format.number(input.stats.parsed ?? input.stats.received - input.stats.parse_failed),
                        },
                        { label: mixed("partial"), value: format.number(input.stats.partial ?? 0) },
                        {
                          label: mixed("error"),
                          value: format.number(input.stats.unclassified ?? input.stats.parse_failed),
                        },
                      ]
                    : []),
                ]}
              />
              {input.stats || input.enabled ? (
                <p className="text-muted-foreground text-sm">
                  {input.stats ? mixed("diagnosticWindow") : t(firstMessage[input.protocol])}
                </p>
              ) : null}
              {input.stats && input.stats.parse_failed > 0 && <p className="text-sm">{t("inspectMapping")}</p>}
            </IngestSection>
            <IngestSection title={t("basicInfo")}>
              <DetailFields
                rows={[
                  {
                    label: t("createdTime"),
                    value: format.dateTime(new Date(input.created_at), { dateStyle: "medium", timeStyle: "short" }),
                  },
                  {
                    label: t("updated"),
                    value: format.dateTime(new Date(input.updated_at), { dateStyle: "medium", timeStyle: "short" }),
                  },
                ]}
              />
            </IngestSection>
          </div>
        </div>
        {!editing && error && (
          <p role="alert" className="text-destructive text-sm">
            {t("operationFailed")}
          </p>
        )}
      </Card>
    </section>
  );
}

function InputEditSheet({
  input,
  boundMappingName,
  pending,
  error,
  onClose,
  onChange,
}: {
  input: ApiInput;
  boundMappingName: string;
  pending: boolean;
  error: boolean;
  onClose: () => void;
  onChange: (body: UpdateInputBody, onSuccess?: () => void) => void;
}) {
  const t = useTranslations("Ingest");
  // Keep draft values relative to this edit session even when the detail query refreshes.
  const [initial] = useState(input);
  const [mappingID, setParsingRuleID] = useState(initial.parsing_rule_id ?? unboundRule);
  const form = useForm({ defaultValues: { name: initial.name, vendor: initial.vendor, product: initial.product } });
  const [name, vendor, product] = useWatch({ control: form.control, name: ["name", "vendor", "product"] });
  const canReadRules = useCan(permissions.IngestParsingRuleRead);
  const canWriteRules = useCan(permissions.IngestParsingRuleWrite);
  const canVerify = canReadRules && canWriteRules;
  const { data: selected } = useMappingDetail(mappingID);
  const trial = useMappingTrial(selected);
  const mappingChanged = canVerify && mappingID !== (initial.parsing_rule_id ?? unboundRule);
  const needsTrial = mappingChanged && mappingID !== unboundRule;
  const changes: UpdateInputBody = {};
  if (name.trim() !== initial.name) changes.name = name.trim();
  if (vendor.trim() !== initial.vendor) changes.vendor = vendor.trim();
  if (product.trim() !== initial.product) changes.product = product.trim();
  if (mappingChanged && mappingID === unboundRule) changes.unbind_rule = true;
  if (needsTrial && selected) {
    changes.parsing_rule_id = selected.id;
    changes.expected_rule_version = selected.version;
  }
  return (
    <SheetPanel
      title={t("editConfig")}
      onClose={() => {
        if (!pending) onClose();
      }}
      footer={
        <>
          <Button variant="outline" onClick={onClose} disabled={pending}>
            {t("cancel")}
          </Button>
          <Button
            disabled={pending || !name.trim() || !Object.keys(changes).length || (needsTrial && !trial.verified)}
            onClick={() => onChange(changes, onClose)}
          >
            {t("save")}
          </Button>
        </>
      }
    >
      <div className="grid min-w-0 items-start gap-6">
        <IngestSection title={t("logSource")}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="input-detail-name">{t("name")}</FieldLabel>
              <Input id="input-detail-name" maxLength={100} {...form.register("name")} />
            </Field>
            <SourceIdentityFields maxLength={120} vendor={form.register("vendor")} product={form.register("product")} />
          </FieldGroup>
        </IngestSection>
        <IngestSection title={t("parseConfig")}>
          {canVerify ? (
            <RulePicker value={mappingID} selected={selected} onChange={setParsingRuleID} disabled={pending} />
          ) : (
            <DetailFields rows={[{ label: t("mapping"), value: boundMappingName }]} />
          )}
          {needsTrial && <SampleTrial trial={trial} />}
        </IngestSection>
      </div>
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {t("operationFailed")}
        </p>
      )}
    </SheetPanel>
  );
}
