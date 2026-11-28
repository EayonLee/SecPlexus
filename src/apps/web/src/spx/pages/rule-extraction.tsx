"use client";

import { useState } from "react";

import { useMutation } from "@tanstack/react-query";
import { BinaryIcon, PlusIcon, ScissorsIcon, Trash2Icon } from "lucide-react";
import { useTranslations } from "next-intl";
import { useFieldArray, useForm, useWatch } from "react-hook-form";
import { z } from "zod";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import type { ExtractionValueType, extractMapping } from "@/spx/api/ingest/mappings";
import { SearchField } from "@/spx/components/search-field";
import { SheetPanel } from "@/spx/components/sheet-panel";

import { type FieldProcessor, fieldOperations } from "./mixed-rule-model";

export function ExtractionFields({
  fields,
  fieldTypes,
  sourcePaths,
  supplements,
  onSupplement,
  preview,
  processors,
  readonly,
  onChange,
}: {
  fields: Record<string, unknown>;
  sourcePaths: string[];
  supplements: Record<string, unknown>;
  onSupplement: (values: Record<string, unknown>) => void;
  preview: (processors: FieldProcessor[], signal: AbortSignal) => ReturnType<typeof extractMapping>;
  fieldTypes?: Record<string, ExtractionValueType>;
  processors: FieldProcessor[];
  readonly: boolean;
  onChange: (processors: FieldProcessor[]) => void;
}) {
  const t = useTranslations("RuleWizard");
  const mt = useTranslations("Mappings");
  const common = useTranslations("Common");
  const ingest = useTranslations("Ingest");
  const [search, setSearch] = useState("");
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<FieldProcessor>();
  const [submitted, setSubmitted] = useState(false);
  const processorPreview = useMutation({
    mutationFn: ({ next, signal }: { next: FieldProcessor[]; edit: FieldProcessor; signal: AbortSignal }) =>
      preview(next, signal),
  });
  const currentPreview = draft && processorPreview.variables?.edit === draft;
  const processed =
    currentPreview && processorPreview.data?.compiled ? processorPreview.data.previews?.[0]?.fields : undefined;
  const ready = !!(draft && processed && Object.hasOwn(processed, draft.target));
  const processorIssue =
    currentPreview &&
    (processorPreview.isError
      ? t("extractionFailed")
      : processorPreview.isSuccess && !ready
        ? t("processorSampleInvalid")
        : undefined);
  const trimming = draft?.operation.startsWith("trim") ?? false;
  const path = draft?.operation === "json" ? draft.path?.trim() : undefined;
  const pathMissing = draft?.operation === "json" && !path;
  let targetProblem: "processorTargetFormat" | "processorTargetConflict" | undefined;
  if (draft) {
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$/.test(draft.target)) targetProblem = "processorTargetFormat";
    else if (
      sourcePaths.includes(draft.target) ||
      Object.hasOwn(fields, draft.target) ||
      processors.some((step) => step.target === draft.target)
    )
      targetProblem = "processorTargetConflict";
  }
  const targetError = submitted && targetProblem ? t(targetProblem) : undefined;
  const pathError = submitted && pathMissing;
  const trimText = draft?.operation === "trim_prefix" || draft?.operation === "trim_suffix";
  const textError = submitted && trimText && !draft.value;
  const entries = Object.entries(fields).filter(([key]) => key.toLowerCase().includes(search.trim().toLowerCase()));
  return (
    <div className="grid min-w-0 gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <SearchField value={search} onChange={setSearch} placeholder={t("searchFields")} />
        {!readonly && (
          <Button
            size="sm"
            variant="outline"
            disabled={Object.keys(supplements).length >= 32}
            onClick={() => setAdding(true)}
          >
            <PlusIcon data-icon="inline-start" /> {t("supplementFields")}
          </Button>
        )}
      </div>
      {processors.length > 0 && (
        <ul className="divide-y rounded-md border px-3">
          {processors.map((step) => (
            <li key={step.target} className="flex items-center justify-between gap-2 py-2 text-sm">
              <span className="min-w-0 break-all">
                <code>{step.source}</code> → <code>{step.target}</code>
                <span className="ml-2 text-muted-foreground">{t(step.operation)}</span>
              </span>
              {!readonly && (
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={t("removeProcessor", { target: step.target })}
                  onClick={() => onChange(processors.filter((item) => item !== step))}
                >
                  <Trash2Icon data-icon="inline-start" />
                  {ingest("delete")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      <div className="overflow-hidden rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{mt("sourceField")}</TableHead>
              <TableHead>{t("extractedValue")}</TableHead>
              {!readonly && <TableHead className="w-40">{t("actions")}</TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map(([key, value]) => {
              const hidden = value === "[REDACTED]";
              const kind = Object.hasOwn(fieldTypes ?? {}, key) ? fieldTypes?.[key] : visibleValueType(value);
              return (
                <TableRow key={key}>
                  <TableCell className="whitespace-normal break-all text-xs">
                    <code>{key}</code>
                    {Object.hasOwn(supplements, key) && !sourcePaths.includes(key) && (
                      <Badge variant="outline" className="ms-2 text-xs">
                        {t("supplementedField")}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="max-w-56 whitespace-normal break-all text-xs">
                    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      <span>{hidden ? t("hidden") : value === null ? t("nullValue") : renderValue(value)}</span>
                      {kind && (
                        <Badge variant="secondary" className="value-type-badge" data-value-type={kind}>
                          {kind}
                        </Badge>
                      )}
                    </div>
                  </TableCell>
                  {!readonly && (
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {(
                          [
                            { name: "cropField", operation: "trim", Icon: ScissorsIcon },
                            { name: "decodeField", operation: "decode_base64", Icon: BinaryIcon },
                          ] as const
                        ).map(({ name, operation, Icon }) => {
                          const unavailable = kind !== "string" || hidden || Object.hasOwn(supplements, key);
                          return (
                            <span key={name} title={unavailable ? t("textOperationOnly") : undefined}>
                              <Button
                                size="sm"
                                variant="ghost"
                                disabled={unavailable || processors.length >= 8}
                                onClick={() => {
                                  setSubmitted(false);
                                  setDraft({ source: key, target: "", operation });
                                }}
                              >
                                <Icon data-icon="inline-start" />
                                {t(name)}
                              </Button>
                            </span>
                          );
                        })}
                        {Object.hasOwn(supplements, key) && !sourcePaths.includes(key) && (
                          <Button
                            size="sm"
                            variant="ghost"
                            aria-label={t("removeSupplement", { field: key })}
                            onClick={() =>
                              onSupplement(
                                Object.fromEntries(Object.entries(supplements).filter(([name]) => name !== key)),
                              )
                            }
                          >
                            <Trash2Icon data-icon="inline-start" />
                            {ingest("delete")}
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  )}
                </TableRow>
              );
            })}
            {!entries.length && (
              <TableRow>
                <TableCell colSpan={readonly ? 2 : 3} className="py-6 text-muted-foreground">
                  {mt("noFields")}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      {adding && (
        <SupplementSheet
          sourcePaths={[...sourcePaths, ...Object.keys(supplements)]}
          maximum={32 - Object.keys(supplements).length}
          onClose={() => setAdding(false)}
          onApply={(additional) => {
            onSupplement({ ...supplements, ...additional });
            setAdding(false);
          }}
        />
      )}
      {draft && (
        <SheetPanel
          layout="management"
          title={t(trimming ? "cropField" : "decodeField")}
          onClose={() => {
            if (!processorPreview.isPending) setDraft(undefined);
          }}
          footer={
            <>
              <Button variant="outline" disabled={processorPreview.isPending} onClick={() => setDraft(undefined)}>
                {common("cancel")}
              </Button>
              <Button
                variant="outline"
                type="submit"
                form="rule-field-processor"
                value="preview"
                disabled={processorPreview.isPending}
              >
                {t("previewProcessor")}
              </Button>
              <Button type="submit" form="rule-field-processor" value="apply" disabled={processorPreview.isPending}>
                {t("applyProcessor")}
              </Button>
            </>
          }
        >
          <form
            id="rule-field-processor"
            className="py-4"
            onSubmit={(event) => {
              event.preventDefault();
              setSubmitted(true);
              if (processorPreview.isPending) return;
              if (pathMissing || targetProblem || textError || (trimText && !draft.value)) {
                event.currentTarget
                  .querySelector<HTMLInputElement>(
                    pathMissing
                      ? "#processor-path"
                      : trimText && !draft.value
                        ? "#processor-text"
                        : "#processor-target",
                  )
                  ?.focus();
                return;
              }
              const next = [...processors, { ...draft, path, value: trimText ? draft.value : undefined }];
              const submitter = (event.nativeEvent as SubmitEvent).submitter;
              const applying = submitter instanceof HTMLButtonElement && submitter.value === "apply";
              if (applying && ready) {
                onChange(next);
                setDraft(undefined);
                return;
              }
              processorPreview.mutate(
                { next, edit: draft, signal: AbortSignal.timeout(15_000) },
                {
                  onSuccess: (result) => {
                    if (
                      applying &&
                      result.compiled &&
                      Object.hasOwn(result.previews?.[0]?.fields ?? {}, draft.target)
                    ) {
                      onChange(next);
                      setDraft(undefined);
                    }
                  },
                },
              );
            }}
          >
            <Field className="mb-4">
              <FieldLabel htmlFor="processor-before">{t("processorBefore")}</FieldLabel>
              <Textarea
                id="processor-before"
                variant="three-lines"
                className="bg-muted/40 font-mono"
                readOnly
                value={renderValue(fields[draft.source])}
              />
            </Field>
            <fieldset disabled={processorPreview.isPending}>
              <FieldGroup className="gap-4">
                <Field>
                  <FieldLabel htmlFor="processor-operation">{t("processing")}</FieldLabel>
                  <Select
                    value={draft.operation}
                    onValueChange={(value) => {
                      const operation = fieldOperations.find((item) => item === value);
                      if (operation) setDraft({ ...draft, operation, value: undefined, path: undefined });
                    }}
                  >
                    <SelectTrigger id="processor-operation" className="w-full">
                      <SelectValue>{t(draft.operation)}</SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {fieldOperations
                          .filter((operation) => operation.startsWith("trim") === trimming)
                          .map((operation) => (
                            <SelectItem key={operation} value={operation}>
                              {t(operation)}
                            </SelectItem>
                          ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FieldDescription>{t(`${draft.operation}Hint`)}</FieldDescription>
                </Field>
                {trimText && (
                  <Field data-invalid={textError}>
                    <FieldLabel htmlFor="processor-text">{t("trimText")}</FieldLabel>
                    <Input
                      id="processor-text"
                      value={draft.value ?? ""}
                      maxLength={256}
                      aria-invalid={textError}
                      aria-describedby={textError ? "processor-text-error" : undefined}
                      onChange={(event) => setDraft({ ...draft, value: event.target.value })}
                      className="font-mono"
                    />
                    {textError && <FieldError id="processor-text-error">{t("trimTextRequired")}</FieldError>}
                  </Field>
                )}
                {draft.operation === "json" && (
                  <Field>
                    <FieldLabel htmlFor="processor-path">{t("jsonPath")}</FieldLabel>
                    <Input
                      id="processor-path"
                      value={draft.path ?? ""}
                      placeholder="client.ip"
                      aria-invalid={pathError}
                      aria-describedby={pathError ? "processor-path-error" : undefined}
                      onChange={(event) => setDraft({ ...draft, path: event.target.value })}
                      className="font-mono"
                    />
                    {pathError && <FieldError id="processor-path-error">{t("processorPathRequired")}</FieldError>}
                  </Field>
                )}
                <Field>
                  <FieldLabel htmlFor="processor-target">{t("derivedField")}</FieldLabel>
                  <Input
                    id="processor-target"
                    value={draft.target}
                    placeholder="client_ip"
                    aria-invalid={!!targetError}
                    aria-describedby={targetError ? "processor-target-error" : undefined}
                    onChange={(event) => setDraft({ ...draft, target: event.target.value })}
                    className="font-mono"
                  />
                  {targetError && <FieldError id="processor-target-error">{targetError}</FieldError>}
                </Field>
                {processorIssue && <FieldError role="alert">{processorIssue}</FieldError>}
              </FieldGroup>
            </fieldset>
            {ready && processed && (
              <section aria-labelledby="processor-result" className="mt-4 grid gap-2">
                <h3 id="processor-result" className="text-sm font-medium">
                  {t("processorResult")}
                </h3>
                <Textarea
                  aria-label={t("processorResult")}
                  variant="three-lines"
                  readOnly
                  className="bg-muted/40 font-mono"
                  value={processed[draft.target] === "[REDACTED]" ? t("hidden") : renderValue(processed[draft.target])}
                />
              </section>
            )}
          </form>
        </SheetPanel>
      )}
    </div>
  );
}

export function visibleValueType(value: unknown): ExtractionValueType | undefined {
  if (value === "[REDACTED]") return;
  if (value === null) return "null";
  if (Array.isArray(value)) return "array";
  const kind = typeof value;
  if (kind === "string" || kind === "number" || kind === "boolean" || kind === "object") return kind;
}

export function renderValue(value: unknown) {
  if (value === undefined) return "—";
  return typeof value === "string" ? value : JSON.stringify(value);
}

const valueTypes = ["auto", "string", "number", "boolean", "object", "array", "null"] as const;
const newSupplement = { source: "", value: "", kind: "auto" as (typeof valueTypes)[number] };
const supplementPath = z
  .string()
  .max(128)
  .regex(/^[^\s.]+(\.[^\s.]+)*$/u);
const jsonValue = z.json();

function supplementValue(value: string, kind: (typeof valueTypes)[number]) {
  if (kind === "string") return { value };
  if (!value.length) return kind === "null" ? { value: null } : { value: undefined };
  try {
    const parsed = jsonValue.safeParse(JSON.parse(value));
    if (parsed.success && (kind === "auto" || visibleValueType(parsed.data) === kind)) return { value: parsed.data };
  } catch {
    if (kind === "auto") return { value };
  }
}

function SupplementSheet({
  sourcePaths,
  maximum,
  onClose,
  onApply,
}: {
  sourcePaths: string[];
  maximum: number;
  onClose: () => void;
  onApply: (values: Record<string, unknown>) => void;
}) {
  const t = useTranslations("RuleWizard");
  const mt = useTranslations("Mappings");
  const common = useTranslations("Common");
  const ingest = useTranslations("Ingest");
  const form = useForm({ defaultValues: { sources: [newSupplement] } });
  const { fields, append, remove } = useFieldArray({ control: form.control, name: "sources" });
  const values = useWatch({ control: form.control, name: "sources" });
  return (
    <SheetPanel
      title={t("supplementFields")}
      layout="management"
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {common("cancel")}
          </Button>
          <Button type="submit" form="rule-supplement" disabled={maximum < 1}>
            {t("applySupplement")}
          </Button>
        </>
      }
    >
      <form
        id="rule-supplement"
        className="grid gap-4 py-4"
        onSubmit={form.handleSubmit(({ sources }) => {
          const existing = new Set(sourcePaths);
          const results: [string, unknown][] = [];
          let invalid = false;
          for (const [index, { source, value, kind }] of sources.entries()) {
            const path = source.trim();
            const parsed = supplementValue(value, kind);
            if (!supplementPath.safeParse(path).success || existing.has(path)) {
              form.setError(
                `sources.${index}.source`,
                { message: t(existing.has(path) ? "processorTargetConflict" : "invalidSourcePath") },
                { shouldFocus: !invalid },
              );
              invalid = true;
            } else if (!parsed) {
              form.setError(
                `sources.${index}.value`,
                { message: t("invalidSupplementValue") },
                { shouldFocus: !invalid },
              );
              invalid = true;
            } else {
              results.push([path, parsed.value]);
              existing.add(path);
            }
          }
          if (!invalid && results.length <= maximum) onApply(Object.fromEntries(results));
        })}
      >
        <FieldDescription>{t("supplementHint")}</FieldDescription>
        {fields.map((row, index) => {
          const sourceError = form.formState.errors.sources?.[index]?.source?.message;
          const valueError = form.formState.errors.sources?.[index]?.value?.message;
          return (
            <fieldset key={row.id} className="grid gap-3 rounded-md border p-3">
              <div className="flex items-end gap-2">
                <Field data-invalid={!!sourceError}>
                  <FieldLabel htmlFor={`supplement-field-${index}`}>{mt("sourceField")}</FieldLabel>
                  <Input
                    id={`supplement-field-${index}`}
                    className="font-mono"
                    placeholder="user.name"
                    maxLength={128}
                    aria-invalid={!!sourceError}
                    aria-describedby={sourceError ? `supplement-source-error-${index}` : undefined}
                    {...form.register(`sources.${index}.source`)}
                  />
                  {sourceError && <FieldError id={`supplement-source-error-${index}`}>{sourceError}</FieldError>}
                </Field>
                {fields.length > 1 && (
                  <Button
                    size="sm"
                    variant="ghost"
                    type="button"
                    aria-label={t("removeMapping", { number: index + 1 })}
                    onClick={() => remove(index)}
                  >
                    <Trash2Icon data-icon="inline-start" />
                    {ingest("delete")}
                  </Button>
                )}
              </div>
              <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_8rem]">
                <Field data-invalid={!!valueError}>
                  <FieldLabel htmlFor={`supplement-value-${index}`}>{t("sampleValue")}</FieldLabel>
                  <Input
                    id={`supplement-value-${index}`}
                    className="font-mono"
                    maxLength={4096}
                    aria-invalid={!!valueError}
                    aria-describedby={valueError ? `supplement-value-error-${index}` : undefined}
                    {...form.register(`sources.${index}.value`)}
                  />
                  {valueError && <FieldError id={`supplement-value-error-${index}`}>{valueError}</FieldError>}
                </Field>
                <Field>
                  <FieldLabel htmlFor={`supplement-kind-${index}`}>{t("sampleValueType")}</FieldLabel>
                  <Select
                    value={values[index]?.kind ?? "auto"}
                    onValueChange={(value) => {
                      const kind = valueTypes.find((item) => item === value);
                      if (kind) form.setValue(`sources.${index}.kind`, kind);
                    }}
                  >
                    <SelectTrigger id={`supplement-kind-${index}`} className="w-full">
                      <SelectValue>
                        {values[index]?.kind && values[index].kind !== "auto" ? values[index].kind : t("autoValueType")}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {valueTypes.map((kind) => (
                          <SelectItem key={kind} value={kind}>
                            {kind === "auto" ? t("autoValueType") : kind}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
              </div>
            </fieldset>
          );
        })}
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="justify-self-start"
          disabled={fields.length >= maximum}
          onClick={() => append(newSupplement)}
        >
          <PlusIcon data-icon="inline-start" />
          {t("addSupplement")}
        </Button>
      </form>
    </SheetPanel>
  );
}
