"use client";

import { type ReactNode, useState } from "react";

import { useQuery } from "@tanstack/react-query";
import { CheckIcon, Globe2, PlusIcon, RadioTower, Trash2Icon, Workflow } from "lucide-react";
import { useTranslations } from "next-intl";
import { useFieldArray, useForm, useWatch } from "react-hook-form";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import { getMapping, listMappingPage, mappingKey, validateMapping } from "@/spx/api/ingest/mappings";
import type { ApiMappingDetail, ApiMappingSummary, SampleResult } from "@/spx/api/ingest/schemas";

import { useCan } from "../components/access";
import { MappingPreview } from "../components/ingest-feedback";
import {
  ListSearchContent,
  ListSearchOption,
  ListSearchTrigger,
  useListSearchState,
} from "../components/list-controls";
import { inputTrialKey } from "./ingest-model";
import { parserModes, readRuleCatalog } from "./rule-catalog";
import { useRuleRequest } from "./use-rule-request";

export const channelIcons = { http: Globe2, syslog: RadioTower, kafka: Workflow };
export const protocols = ["http", "syslog", "kafka"] as const;
export const unboundRule = "unbound";
const steps = ["sourceStep", "connectionStep", "sampleWorkspace", "reviewStep"] as const;
export function useMappingDetail(id?: string) {
  const canRead = useCan(permissions.IngestParsingRuleRead);
  return useQuery({
    queryKey: [...mappingKey, "detail", id],
    queryFn: ({ signal }) => getMapping(id ?? "", signal),
    enabled: !!id && id !== unboundRule && canRead,
  });
}

const maxTrialSamples = 20;

export function useMappingTrial(mapping?: ApiMappingDetail) {
  const form = useForm({ defaultValues: { samples: [{ value: "" }] } });
  const entries = useFieldArray({ control: form.control, name: "samples" });
  const samples = useWatch({ control: form.control, name: "samples" });
  const submitted = samples.flatMap(({ value }, index) => value.trim() ? [{ value, index }] : []);
  const key = inputTrialKey(mapping, submitted.map(({ value }) => value));
  const verify = useRuleRequest<SampleResult>(key);
  const result = verify.data;
  const failedRows = new Set(
    [...(result?.failures ?? []), ...(result?.previews ?? []).filter((row) => row.status !== "ok")]
      .map((row) => submitted[row.index]?.index),
  );

  return {
    form, entries, result, failedRows,
    hasSamples: submitted.length > 0,
    verified: !!result?.compiled && result.total === submitted.length && submitted.length > 0 &&
      result.previews?.length === submitted.length && failedRows.size === 0,
    pending: verify.isPending,
    failed: verify.isError,
    run: () => {
      if (mapping) void verify.run(key, (signal) => validateMapping(
        mapping.definition,
        submitted.map(({ value }) => value),
        { vendor: mapping.vendor, product: mapping.product, parsing_rule_id: mapping.id, expected_version: mapping.version },
        signal,
      ));
    },
  };
}

export function SampleTrial({ trial }: { trial: ReturnType<typeof useMappingTrial> }) {
  const t = useTranslations("Mappings");
  const wizard = useTranslations("RuleWizard");
  const shared = useTranslations("Ingest");
  const { entries, form } = trial;
  return (
    <FieldGroup className="gap-3">
      <div className="flex items-center justify-between gap-2">
        <FieldLabel htmlFor={`input-trial-${entries.fields[0]?.id}`}>{t("samples")}</FieldLabel>
        <span className="text-xs text-muted-foreground tabular-nums">
          {wizard("sampleCount", { count: entries.fields.length, total: maxTrialSamples })}
        </span>
      </div>
      {entries.fields.map((field, index) => {
        const id = `input-trial-${field.id}`;
        const errorID = trial.failedRows.has(index) ? `${id}-error` : undefined;
        return (
          <Field key={field.id} orientation="horizontal" className="items-start gap-2" data-invalid={!!errorID}>
            <div className="grid min-w-0 flex-1 gap-1.5">
              <Textarea
                id={id}
                aria-label={entries.fields.length === 1 ? t("samples") : t("sampleNumber", { number: index + 1 })}
                aria-invalid={!!errorID}
                aria-describedby={errorID}
                variant="three-lines"
                className="font-mono"
                placeholder={wizard("samplePlaceholder")}
                spellCheck={false}
                {...form.register(`samples.${index}.value`)}
              />
              {errorID && <FieldError id={errorID}>{wizard("sampleVerificationFailed")}</FieldError>}
            </div>
            {entries.fields.length > 1 && (
              <Button type="button" variant="ghost" size="icon-sm" aria-label={wizard("removeSample", { number: index + 1 })}
                onClick={() => entries.remove(index)}>
                <Trash2Icon />
              </Button>
            )}
          </Field>
        );
      })}
      <div className="flex flex-wrap gap-2">
        <Button type="button" size="sm" variant="outline" disabled={entries.fields.length >= maxTrialSamples}
          onClick={() => entries.append({ value: "" })}>
          <PlusIcon data-icon="inline-start" />{wizard("addSample")}
        </Button>
        <Button type="button" size="sm" variant="outline" disabled={!trial.hasSamples || trial.pending} onClick={trial.run}>
          {t("verify")}
        </Button>
      </div>
      {trial.failed && <p role="alert" className="text-destructive text-sm">{shared("operationFailed")}</p>}
      {trial.result && <MappingPreview result={trial.result} />}
    </FieldGroup>
  );
}

export function WizardProgress({
  step,
  onNavigate,
  labels,
  title,
  reachable = step,
}: {
  step: number;
  onNavigate?: (index: number) => void;
  labels?: string[];
  title?: string;
  reachable?: number;
}) {
  const t = useTranslations("MixedRule");
  const shared = useTranslations("Ingest");
  return (
    <ol aria-label={title ?? shared("createInput")} className="grid grid-cols-2 gap-x-3 gap-y-2 sm:grid-cols-4">
      {(labels ?? steps.map((label) => t(label))).map((label, index) => {
        const segment = `flex min-h-11 w-full items-center justify-center gap-2 rounded-none border-0 border-b-2 px-2 py-2 text-center text-sm font-medium ${index <= step ? "border-primary text-foreground" : "border-border text-muted-foreground"}`;
        const content = (
          <>
            <span
              aria-hidden
              className={`grid size-6 shrink-0 place-items-center rounded-full text-xs tabular-nums ${index === step ? "bg-primary text-primary-foreground" : "bg-muted"}`}
            >
              {index < step ? <CheckIcon className="size-3.5" /> : index + 1}
            </span>
            {label}
          </>
        );
        return (
          <li key={label} aria-current={index === step ? "step" : undefined}>
            {index !== step && index <= reachable && onNavigate ? (
              <Button
                variant="ghost"
                type="button"
                onClick={() => onNavigate(index)}
                className={`${segment} h-full whitespace-normal`}
              >
                {content}
              </Button>
            ) : (
              <span className={`${segment} h-full`}>{content}</span>
            )}
          </li>
        );
      })}
    </ol>
  );
}

export function RulePicker({
  value,
  selected,
  onChange,
  disabled,
  filterMode = false,
}: {
  value: string;
  selected?: ApiMappingDetail;
  onChange: (id: string, rule?: ApiMappingSummary) => void;
  disabled?: boolean;
  filterMode?: boolean;
}) {
  const t = useTranslations("Ingest");
  const common = useTranslations("Common");
  const mappings = useTranslations("Mappings");
  const search = useListSearchState();
  const [lastPicked, setLastPicked] = useState<ApiMappingSummary>();
  const keyword = search.text?.trim() ?? "";
  const page = useQuery({
    queryKey: [...mappingKey, "picker", keyword],
    queryFn: ({ signal }) => listMappingPage({ limit: 50, q: keyword }, signal),
    enabled: search.open && !disabled,
  });
  const empty = {
    id: filterMode ? "" : unboundRule,
    name: filterMode ? common("allOptions") : t("mappingNone"),
    meta: "",
    parserType: "",
  };
  function option(rule: ApiMappingSummary) {
    if (filterMode) return { id: rule.id, name: rule.name, meta: "", parserType: "" };
    const mode = parserModes.find((value) => value === rule.parse_type);
    const parserType = mode ? mappings(mode) : "";
    return {
      id: rule.id,
      name: rule.name,
      meta: [rule.vendor, rule.product, parserType].filter(Boolean).join(" · "),
      parserType,
    };
  }
  const options = [
    ...(!keyword || empty.name.toLowerCase().includes(keyword.toLowerCase()) ? [empty] : []),
    ...(page.data?.items ?? []).map(option),
  ];
  const known = selected ?? lastPicked;
  let selection = value === empty.id ? empty : options.find((item) => item.id === value);
  if (!selection && known?.id === value) {
    selection = option({
      ...known,
      parse_type: selected ? readRuleCatalog(selected.definition).mode : lastPicked?.parse_type,
    });
  }
  const label = (item: (typeof options)[number]) =>
    filterMode ? item.name : [item.name, item.parserType].filter(Boolean).join(" · ");

  const results = (
    <>
      <ComboboxList>
        {(item: (typeof options)[number]) =>
          filterMode ? (
            <ListSearchOption key={item.id} value={item} label={item.name} />
          ) : (
            <ComboboxItem key={item.id} value={item}>
              <span className="grid min-w-0">
                <span className="break-all">{item.name}</span>
                {item.meta && <small className="text-muted-foreground">{item.meta}</small>}
              </span>
            </ComboboxItem>
          )
        }
      </ComboboxList>
      <ComboboxEmpty>{page.isFetching ? common("loading") : t(page.isError ? "loadFailed" : "noRules")}</ComboboxEmpty>
    </>
  );
  return (
    <div className={filterMode ? "min-w-0 max-w-full" : "grid gap-2"}>
      {!filterMode && <FieldLabel htmlFor="input-rule-picker">{t("mapping")}</FieldLabel>}
      <Combobox
        items={selection && !options.some((item) => item.id === selection.id) ? [...options, selection] : options}
        filteredItems={options}
        value={selection ?? null}
        inputValue={search.text ?? (!filterMode && selection ? label(selection) : "")}
        disabled={disabled}
        openOnInputClick={!filterMode}
        onOpenChange={search.onOpenChange}
        onInputValueChange={search.onInputValueChange}
        isItemEqualToValue={(item, current) => item.id === current.id}
        itemToStringLabel={label}
        filter={null}
        onValueChange={(item) => {
          if (!item) return;
          const rule = page.data?.items.find((candidate) => candidate.id === item.id);
          if (rule) setLastPicked(rule);
          onChange(item.id, rule);
        }}
      >
        {filterMode ? (
          <>
            <ListSearchTrigger label={t("mapping")} value={selection?.name} />
            <ListSearchContent label={t("mapping")}>{results}</ListSearchContent>
          </>
        ) : (
          <>
            <ComboboxInput
              id="input-rule-picker"
              className="w-full"
              placeholder={t("selectMapping")}
              disabled={disabled}
              maxLength={255}
            />
            <ComboboxContent>{results}</ComboboxContent>
          </>
        )}
      </Combobox>
    </div>
  );
}

export function DetailFields({ rows }: { rows: { label: string; value: ReactNode }[] }) {
  return (
    <dl className="grid gap-x-6 gap-y-4 text-sm sm:grid-cols-2">
      {rows.map((row) => (
        <div key={row.label} className="flex min-w-0 flex-col gap-1">
          <dt className="text-xs text-muted-foreground">{row.label}</dt>
          <dd className="min-w-0 break-words">{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}
