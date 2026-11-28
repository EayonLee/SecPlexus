"use client";

import { useState } from "react";

import { PlusIcon, Trash2Icon } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { z } from "zod";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { CatalogField, EventClassDetails, EventField } from "@/spx/api/ingest/event-schema";
import type { ExtractionValueType } from "@/spx/api/ingest/mappings";

import type { MixedDefinition } from "./mixed-rule-model";
import { mappingFieldType } from "./rule-catalog";
import { RuleFieldPicker } from "./rule-field-picker";

export type MappingRow = {
  id: string;
  source: string;
  target: string;
  fixed: boolean;
  conversion: string;
  location?: string;
  fallback?: string;
};

export function newMappingRow(source = ""): MappingRow {
  return { id: crypto.randomUUID(), source, target: "", fixed: false, conversion: "" };
}

export function duplicateMappingTargets(rows: Pick<MappingRow, "target">[]) {
  const targets = new Set<string>();
  const duplicates = new Set<string>();
  for (const { target } of rows) {
    if (!target) continue;
    if (targets.has(target)) duplicates.add(target);
    targets.add(target);
  }
  return duplicates;
}

export function mappingRows(shared?: MixedDefinition["shared"]): MappingRow[] {
  if (!shared) return [];
  const rows: MappingRow[] = Object.entries(shared.field_map).map(([source, target]) => ({
    id: target,
    source,
    target,
    fixed: false,
    conversion: formatMap(shared.value_map?.[target]),
    fallback: Object.hasOwn(shared.defaults ?? {}, target)
      ? typeof shared.defaults?.[target] === "string"
        ? String(shared.defaults[target])
        : JSON.stringify(shared.defaults?.[target])
      : undefined,
  }));
  for (const [target, value] of Object.entries(shared.constants ?? {})) {
    rows.push({
      id: target,
      target,
      source: "",
      fallback: typeof value === "string" ? value : JSON.stringify(value),
      fixed: true,
      conversion: "",
    });
  }
  if (shared.time?.field)
    rows.push({
      id: "time",
      source: shared.time.field,
      target: "time",
      fixed: false,
      conversion: shared.time.formats?.join(", ") ?? "",
      location: shared.time.location,
    });
  if (shared.severity?.field || shared.severity?.default !== undefined)
    rows.push({
      id: "severity_id",
      source: shared.severity.field ?? "",
      fallback: shared.severity.default === undefined ? undefined : String(shared.severity.default),
      target: "severity_id",
      fixed: false,
      conversion: formatMap(shared.severity.values),
    });
  return rows;
}

function formatMap(value: Record<string, number> | undefined) {
  return Object.entries(value ?? {})
    .map(([name, code]) => `${name}=${code}`)
    .join(", ");
}

function parseValueMap(raw: string) {
  const entries: [string, number][] = [];
  for (const pair of raw.split(",")) {
    const matches = /^(.+?)=\s*(-?\d+)\s*$/.exec(pair.trim());
    if (!matches || entries.some(([name]) => name === matches[1].trim())) return;
    entries.push([matches[1].trim(), Number(matches[2])]);
  }
  return Object.fromEntries(entries);
}

export function sharedMapping(rows: MappingRow[], fields: EventField[]): MixedDefinition["shared"] {
  if (duplicateMappingTargets(rows).size) return;
  const compiled: NonNullable<MixedDefinition["shared"]> = { field_map: {} };
  for (const row of rows) {
    const field = fields.find((item) => item.path === row.target);
    const source = row.source.trim();
    if (!field) return;
    const hasDefault = row.fixed || (row.fallback !== undefined && row.fallback !== "");
    const fallback = hasDefault ? typedValue(field, row.fallback ?? "") : undefined;
    if (hasDefault && fallback === undefined) return;
    if (row.target === "time") {
      const formats = row.conversion
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean);
      if (!source || !formats.length || hasDefault) return;
      compiled.time = { field: source, formats, location: row.location?.trim() || undefined };
    } else if (row.target === "severity_id") {
      const values = row.conversion.trim()
        ? parseValueMap(row.conversion)
        : Object.fromEntries((field.enum ?? []).map(({ value }) => [String(value), value]));
      if (!values || (!source && !hasDefault) || fallback === 99) return;
      compiled.severity = {
        field: source || undefined,
        values: source ? values : undefined,
        default: typeof fallback === "number" ? fallback : undefined,
      };
    } else if (!source) {
      if (!hasDefault) return;
      compiled.constants = { ...compiled.constants, [row.target]: fallback };
    } else {
      if (Object.hasOwn(compiled.field_map, source)) return;
      compiled.field_map = { ...compiled.field_map, [source]: row.target };
      if (hasDefault) compiled.defaults = { ...compiled.defaults, [row.target]: fallback };
      if (row.conversion.trim()) {
        const values = parseValueMap(row.conversion);
        if (
          !values ||
          (field.enum && Object.values(values).some((code) => !field.enum?.some(({ value }) => code === value)))
        )
          return;
        compiled.value_map = { ...compiled.value_map, [row.target]: values };
      }
    }
  }
  return compiled;
}

function typedValue(field: EventField, input: string): unknown {
  let schema: z.ZodType = z.json();
  switch (field.json_type) {
    case "string":
      schema = field.logical_type === "ip_t" ? z.union([z.ipv4(), z.ipv6()]) : z.string();
      if (!field.array) return schema.safeParse(input).data;
      break;
    case "boolean":
      schema = z.boolean();
      break;
    case "object":
      schema = z.record(z.string(), z.json());
      break;
    case "integer":
      schema = field.integer_range
        ? z.int().min(field.integer_range.min).max(field.integer_range.max)
        : field.logical_type === "port_t"
          ? z.int().min(0).max(65535)
          : z.int();
      if (field.enum) schema = schema.refine((value) => field.enum?.some((entry) => entry.value === value) ?? false);
      break;
    case "number":
      schema = z.number();
      break;
  }
  try {
    return (field.array ? z.array(schema).max(64) : schema).safeParse(JSON.parse(input)).data;
  } catch {
    return;
  }
}

export function missingMappingGroups(rows: MappingRow[], constraints: EventClassDetails["constraints"]) {
  const outputs = rows.filter((row) => row.target && (row.source.trim() || row.fallback));
  return constraints.filter((group) => {
    if (group.when && !outputs.some((row) => row.target === group.when || row.target.startsWith(group.when + ".")))
      return false;
    const present = group.paths.filter((path) =>
      outputs.some(
        (row) => group.mapping_paths.includes(row.target) && (row.target === path || row.target.startsWith(path + ".")),
      ),
    );
    return present.length === 0 || (group.mode === "just_one" && present.length !== 1);
  });
}

export function mappingPresets(
  fields: EventField[],
  rows: MappingRow[],
  constraints: EventClassDetails["constraints"],
  recommendations: string[],
) {
  const assignable = new Set(
    fields
      .filter((field) => field.owner === "mapping" && (field.json_type !== "object" || field.object_fields?.length))
      .map((field) => field.path),
  );
  const core = new Set(
    fields
      .filter(
        (field) =>
          field.path === "time" ||
          field.path === "severity_id" ||
          (assignable.has(field.path) && field.requirement === "required" && !field.path.includes(".")),
      )
      .map((field) => field.path),
  );
  for (const group of constraints) {
    if (group.when && !rows.some((row) => row.target === group.when || row.target.startsWith(`${group.when}.`)))
      continue;
    for (const path of group.paths) {
      const candidates = group.mapping_paths.filter(
        (target) => assignable.has(target) && (target === path || target.startsWith(`${path}.`)),
      );
      const visible = recommendations.find((target) => candidates.includes(target)) ?? candidates[0];
      if (visible) core.add(visible);
    }
  }
  return { core, recommended: recommendations.filter((path) => assignable.has(path) && !core.has(path)) };
}

export function RuleMappingRows({
  rows,
  fields,
  classes,
  presets,
  dismissed = [],
  onDismiss,
  missing = [],
  sources,
  sourceTypes,
  readOnly,
  submittedRowIDs,
  onChange,
}: {
  rows: MappingRow[];
  fields: CatalogField[];
  classes: { uid: number; name: string }[];
  presets: ReturnType<typeof mappingPresets>;
  dismissed?: string[];
  onDismiss: (target: string) => void;
  missing?: EventClassDetails["constraints"];
  sources: string[];
  sourceTypes: ReadonlyMap<string, ExtractionValueType>;
  readOnly: boolean;
  submittedRowIDs?: Set<string>;
  onChange: (rows: MappingRow[]) => void;
}) {
  const t = useTranslations("RuleWizard");
  const mt = useTranslations("Mappings");
  const common = useTranslations("Ingest");
  const locale = useLocale();
  const [adding, setAdding] = useState(false);
  const duplicateTargets = duplicateMappingTargets(rows);
  const presetPaths = [...presets.core, ...presets.recommended.filter((path) => !dismissed.includes(path))];
  const displayed = [
    ...presetPaths.flatMap((target) =>
      fields.some((field) => field.path === target)
        ? [
            rows.find((row) => row.target === target) ?? {
              id: target,
              source: "",
              target,
              conversion: "",
              fixed: false,
            },
          ]
        : [],
    ),
    ...rows.filter((row) => !presetPaths.includes(row.target)),
  ];
  const available = fields.filter(
    (field) =>
      !displayed.some((row) => row.target === field.path) &&
      !displayed.some((row) => row.target.startsWith(field.path + ".") || field.path.startsWith(row.target + ".")),
  );
  const columns = "@min-[40rem]/mappings:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)_minmax(7rem,.63fr)_3.5rem]";
  return (
    <div className="@container/mappings flex min-w-0 flex-col gap-3">
      {(displayed.length > 0 || adding) && (
        <div className="divide-y rounded-md border">
          <div
            className={`text-muted-foreground hidden gap-3 px-3 py-2 text-xs font-medium @min-[40rem]/mappings:grid ${columns}`}
          >
            <span>{t("standardField")}</span>
            <span>{mt("sourceField")}</span>
            <span>{t("defaultValue")}</span>
            <span />
          </div>
          {displayed.map((row, index) => {
            const field = fields.find((item) => item.path === row.target);
            const targetType = field && mappingFieldType(field);
            const configured = rows.some((item) => item.id === row.id);
            const locked = presets.core.has(row.target);
            const checked = submittedRowIDs?.has(row.id);
            const source = row.source.trim();
            const sourceType = sourceTypes.get(source);
            const typeMismatch =
              targetType &&
              sourceType &&
              sourceType !== targetType.kind &&
              !((targetType.kind === "number" && sourceType === "string") || (field?.enum && row.conversion));
            const special = row.target === "time" || row.target === "severity_id";
            const hasDefault = row.fixed || (row.fallback !== undefined && row.fallback !== "");
            const collision =
              source &&
              !special &&
              rows.some(
                (item) =>
                  item.id !== row.id && item.source.trim() === source && !["time", "severity_id"].includes(item.target),
              );
            const badDefault = !!field && hasDefault && typedValue(field, row.fallback ?? "") === undefined;
            const badMapping = duplicateTargets.has(row.target) || (checked && configured && !source && !hasDefault);
            const badSource =
              !readOnly &&
              (collision || badMapping || missing.some((group) => group.mapping_paths.includes(row.target)));
            const failedGroup = missing.find((group) => group.mapping_paths.includes(row.target));
            const sourceError = badSource ? `mapping-source-error-${index}` : undefined;
            const defaultError = badDefault ? `mapping-default-error-${index}` : undefined;
            const change = (patch: Partial<MappingRow>) => {
              const next = { ...row, ...patch };
              const emptyPreset = presetPaths.includes(row.target) && !next.source.trim() && !next.fallback;
              onChange(
                emptyPreset
                  ? rows.filter((item) => item.id !== row.id)
                  : configured
                    ? rows.map((item) => (item.id === row.id ? next : item))
                    : [...rows, next],
              );
            };
            return (
              <div
                key={row.id}
                data-mapping-path={row.target}
                className={`grid min-w-0 grid-cols-[minmax(0,1fr)_minmax(0,.72fr)_auto] items-start gap-3 px-3 py-2.5 ${columns}`}
              >
                <div className="col-span-3 grid min-w-0 grid-cols-[minmax(6rem,.7fr)_minmax(0,1.3fr)_4.75rem] items-center gap-2 text-sm @min-[40rem]/mappings:col-span-1 @min-[40rem]/mappings:pt-1.5">
                  <span>{field?.name[locale] ?? row.target}</span>
                  <code title={row.target} className="truncate text-xs text-muted-foreground">
                    {row.target}
                  </code>
                  {targetType && (
                    <Badge
                      variant="secondary"
                      className="value-type-badge justify-self-end"
                      data-value-type={targetType.kind}
                    >
                      {targetType.label}
                    </Badge>
                  )}
                </div>
                <Field className="min-w-0 gap-1.5" data-invalid={badSource} title={row.source || undefined}>
                  <FieldLabel className="@min-[40rem]/mappings:sr-only">{mt("sourceField")}</FieldLabel>
                  <Combobox
                    disabled={readOnly}
                    items={sources}
                    value={row.source || null}
                    inputValue={row.source}
                    onInputValueChange={(source, event) => {
                      if (event.reason === "input-change") change({ source, fixed: false });
                    }}
                    onValueChange={(source) => change({ source: source ?? "", fixed: false })}
                  >
                    <ComboboxInput
                      disabled={readOnly}
                      aria-label={`${mt("sourceField")} · ${field?.name[locale] ?? index + 1}`}
                      aria-invalid={!!sourceError}
                      aria-describedby={sourceError ?? (typeMismatch ? `mapping-type-description-${index}` : undefined)}
                      placeholder={t("chooseSource")}
                      className="w-full font-mono [&>input]:truncate"
                      showClear={!readOnly}
                    >
                      {sourceType && (
                        <Badge
                          variant="secondary"
                          className="value-type-badge pointer-events-none ml-1 shrink-0"
                          data-value-type={sourceType}
                        >
                          {sourceType}
                        </Badge>
                      )}
                    </ComboboxInput>
                    <ComboboxContent>
                      <ComboboxEmpty>{mt("noFields")}</ComboboxEmpty>
                      <ComboboxList>
                        {(source: string) => (
                          <ComboboxItem
                            key={source}
                            value={source}
                            aria-label={sourceTypes.has(source) ? `${source} ${sourceTypes.get(source)}` : source}
                          >
                            <span className="grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-3">
                              <code className="truncate">{source}</code>
                              {sourceTypes.get(source) && (
                                <Badge
                                  variant="secondary"
                                  className="value-type-badge"
                                  data-value-type={sourceTypes.get(source)}
                                >
                                  {sourceTypes.get(source)}
                                </Badge>
                              )}
                            </span>
                          </ComboboxItem>
                        )}
                      </ComboboxList>
                    </ComboboxContent>
                  </Combobox>
                  {sourceError && (
                    <FieldError id={sourceError}>
                      {t(
                        collision
                          ? "duplicateSource"
                          : badMapping
                            ? "sourceOrDefault"
                            : failedGroup
                              ? "requiredGroup"
                              : "chooseSource",
                      )}
                    </FieldError>
                  )}
                  {!sourceError && typeMismatch && !readOnly && (
                    <FieldDescription id={`mapping-type-description-${index}`}>
                      {t("sourceTypeMismatch")}
                    </FieldDescription>
                  )}
                  {field &&
                    source &&
                    (!readOnly || row.conversion || row.location) &&
                    (row.target === "time" || field.enum) && (
                      <details open={row.target === "time" && !row.conversion}>
                        <summary className="cursor-pointer text-xs text-muted-foreground">
                          {t(row.target === "time" ? "timeFormat" : "convertValues")}
                        </summary>
                        <div className="mt-2 grid gap-2">
                          <Input
                            aria-label={t(row.target === "time" ? "timeFormat" : "convertValues")}
                            value={row.conversion}
                            readOnly={readOnly}
                            placeholder={row.target === "time" ? "unix_auto" : "info=1, high=4"}
                            onChange={(event) => change({ conversion: event.target.value })}
                          />
                          {row.target === "time" && (
                            <Input
                              aria-label={t("timeZone")}
                              placeholder="UTC"
                              value={row.location ?? ""}
                              readOnly={readOnly}
                              onChange={(event) => change({ location: event.target.value })}
                            />
                          )}
                        </div>
                      </details>
                    )}
                </Field>
                <Field className="min-w-0 gap-1.5" data-invalid={badDefault}>
                  <FieldLabel className="@min-[40rem]/mappings:sr-only">{t("defaultValue")}</FieldLabel>
                  <MappingDefault
                    field={field}
                    label={`${t("defaultValue")} · ${field?.name[locale] ?? index + 1}`}
                    value={row.fallback || (["activity_id", "severity_id"].includes(row.target) ? "0" : "")}
                    errorID={defaultError}
                    readOnly={readOnly}
                    onChange={(fallback) => change({ fallback, fixed: false })}
                  />
                  {defaultError && <FieldError id={defaultError}>{t("invalidDefault")}</FieldError>}
                </Field>
                {!readOnly && !locked && (configured || presets.recommended.includes(row.target)) && (
                  <Button
                    size="sm"
                    variant="ghost"
                    className="justify-self-start px-1"
                    aria-label={t("removeMapping", { number: index + 1 })}
                    onClick={() => {
                      if (presets.recommended.includes(row.target)) onDismiss(row.target);
                      if (configured) onChange(rows.filter((item) => item.id !== row.id));
                    }}
                  >
                    <Trash2Icon data-icon="inline-start" />
                    {common("delete")}
                  </Button>
                )}
              </div>
            );
          })}
          {adding && (
            <fieldset
              className={`grid min-w-0 grid-cols-[minmax(0,1fr)_minmax(0,.72fr)_auto] items-start gap-3 px-3 py-2.5 ${columns}`}
              aria-label={t("addMapping")}
            >
              <div className="col-span-3 @min-[40rem]/mappings:col-span-1">
                <RuleFieldPicker
                  fields={available}
                  value=""
                  label={t("chooseField")}
                  classes={classes}
                  focusOnMount
                  onChange={(target) => {
                    if (!target) return;
                    onChange([...rows, { ...newMappingRow(), target }]);
                    setAdding(false);
                  }}
                />
              </div>
              <Input aria-label={mt("sourceField")} disabled />
              <Input aria-label={t("defaultValue")} disabled />
              <Button size="sm" variant="ghost" className="justify-self-start px-1" onClick={() => setAdding(false)}>
                <Trash2Icon data-icon="inline-start" />
                {common("delete")}
              </Button>
            </fieldset>
          )}
        </div>
      )}
      {!readOnly && (
        <div>
          <Button
            variant="outline"
            size="sm"
            disabled={adding || available.length === 0}
            onClick={() => setAdding(true)}
          >
            <PlusIcon data-icon="inline-start" />
            {t("addMapping")}
          </Button>
        </div>
      )}
    </div>
  );
}

function MappingDefault({
  field,
  label,
  value,
  errorID,
  readOnly,
  onChange,
}: {
  field?: EventField;
  label: string;
  value: string;
  errorID?: string;
  readOnly: boolean;
  onChange: (value: string) => void;
}) {
  const t = useTranslations("RuleWizard");
  const locale = useLocale();
  const [enumSearch, setEnumSearch] = useState("");
  const choices = !field?.array
    ? (field?.enum
        ?.filter((item) => field.path !== "severity_id" || item.value !== 99)
        .map((item) => ({ value: String(item.value), label: item.name[locale] })) ??
      (field?.json_type === "boolean"
        ? [
            { value: "true", label: "true" },
            { value: "false", label: "false" },
          ]
        : undefined))
    : undefined;
  const selected = choices?.find((item) => item.value === value) ?? null;
  const input = { "aria-label": label, "aria-invalid": !!errorID, "aria-describedby": errorID, className: "w-full" };

  if (field?.path === "time") {
    return (
      <Select value="received" disabled={readOnly}>
        <SelectTrigger {...input}>
          <SelectValue>{t("receiptTime")}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="received">{t("receiptTime")}</SelectItem>
        </SelectContent>
      </Select>
    );
  }

  if (choices && field?.preferred_enum_values) {
    const labels = new Map(choices.map((choice) => [choice.value, choice.label]));
    const defaults = field.preferred_enum_values.map(String);
    const preferred = defaults.filter((code) => labels.has(code));
    if (selected && !preferred.includes(selected.value)) preferred.unshift(selected.value);
    return (
      <Combobox
        disabled={readOnly}
        items={enumSearch ? [...labels.keys()] : preferred}
        value={selected?.value ?? null}
        itemToStringLabel={(item) => labels.get(item) ?? item}
        onInputValueChange={(search, event) => {
          if (event.reason === "input-change") setEnumSearch(search.trim());
        }}
        onOpenChange={(open) => {
          if (!open) setEnumSearch("");
        }}
        onValueChange={(next) => onChange(next ?? "")}
      >
        <ComboboxInput {...input} disabled={readOnly} showClear={!readOnly} />
        <ComboboxContent className="max-h-64">
          <ComboboxEmpty>{t("noMatchingValue")}</ComboboxEmpty>
          <ComboboxList>
            {(item: string) => (
              <ComboboxItem key={item} value={item}>
                {labels.get(item)}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    );
  }
  if (choices) {
    return (
      <Select
        disabled={readOnly}
        value={value || "unset"}
        onValueChange={(next) => onChange(next === "unset" ? "" : (next ?? ""))}
      >
        <SelectTrigger {...input}>
          <SelectValue>{selected?.label ?? ""}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          {field?.path !== "severity_id" && <SelectItem value="unset">{t("unsetValue")}</SelectItem>}
          {choices.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }
  if (field?.value_suggestions) {
    const labels = new Map(field.value_suggestions.map((item) => [String(item.value), item.label]));
    const suggestions = [...labels.keys()];
    if (value && !labels.has(value) && typedValue(field, value) !== undefined) suggestions.push(value);
    return (
      <Combobox
        disabled={readOnly}
        items={suggestions}
        value={value || null}
        inputValue={value}
        filter={(item, search) =>
          item.includes(search.trim()) || labels.get(item)?.toLowerCase().includes(search.trim().toLowerCase()) === true
        }
        onInputValueChange={(next, event) => {
          if (event.reason === "input-change") onChange(next);
        }}
        onValueChange={(next) => onChange(next ?? "")}
      >
        <ComboboxInput
          {...input}
          disabled={readOnly}
          inputMode="numeric"
          className="w-full font-mono"
          showClear={!readOnly}
        />
        <ComboboxContent className="max-h-64">
          <ComboboxEmpty>{t("noMatchingValue")}</ComboboxEmpty>
          <ComboboxList>
            {(item: string) => (
              <ComboboxItem value={item} key={item}>
                <code>{item}</code> {labels.get(item)}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    );
  }
  return (
    <Input
      {...input}
      readOnly={readOnly}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="w-full font-mono"
    />
  );
}
