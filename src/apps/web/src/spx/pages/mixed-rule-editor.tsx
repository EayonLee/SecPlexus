"use client";

import { useEffect, useRef, useState } from "react";

import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDownIcon, ArrowLeftIcon, ArrowUpIcon, ChevronDownIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { useFormatter, useLocale, useTranslations } from "next-intl";
import { useFieldArray, useForm, useWatch } from "react-hook-form";

import { permissions } from "@/auth/permissions.gen";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { APIError } from "@/spx/api/client";
import {
  type CatalogField,
  type EventField,
  getEventClass,
  listEventClasses,
  listEventFields,
} from "@/spx/api/ingest/event-schema";
import { mappingInputCounts } from "@/spx/api/ingest/inputs";
import {
  createMapping,
  extractMapping,
  updateMapping,
  validateMapping,
} from "@/spx/api/ingest/mappings";
import {
  isJSONLogSample,
  type RuleTextField,
  ruleMetadata,
  ruleTextFields,
  ruleTextLimits,
} from "@/spx/api/ingest/rule-validation";
import type { ApiMappingDetail, SampleResult } from "@/spx/api/ingest/schemas";

import { useCan } from "../components/access";
import { FormInput } from "../components/forms";
import { IngestConfirm } from "../components/ingest-feedback";
import { IngestSection, SourceIdentityFields } from "../components/ingest-section";
import { PageHeader } from "../components/page-header";
import { WizardProgress } from "./ingest-input-controls";
import { type BranchValue, branchValue, MixedRuleBranch } from "./mixed-rule-branch";
import { type MixedDefinition, mixedDefinition, newBranch, readEventValue } from "./mixed-rule-model";
import { RuleRevisions } from "./rule-revisions";
import { parserModes } from "./rule-catalog";
import { RuleEventTypePicker } from "./rule-event-type-picker";
import { ExtractionFields, visibleValueType } from "./rule-extraction";
import {
  type MappingRow,
  mappingPresets,
  mappingRows,
  missingMappingGroups,
  RuleMappingRows,
  sharedMapping,
} from "./rule-mapping-rows";
import { useRuleRequest } from "./use-rule-request";

const maxRuleSamples = 20;

function ruleFieldProblems(error: unknown) {
  if (!(error instanceof APIError)) return [];
  return error.fieldErrors.flatMap(({ field, code }) => {
    const parsed = ruleTextFields.safeParse(field === "definition.description" ? "description" : field);
    if (
      !parsed.success ||
      !["required", "too_long", "invalid_characters", "invalid_type", "duplicate_name"].includes(code)
    )
      return [];
    return [{ field: parsed.data, code }];
  });
}

function sameFieldMeaning(field: EventField | undefined, other: EventField | undefined) {
  return (
    !!field &&
    !!other &&
    field.owner === other.owner &&
    field.logical_type === other.logical_type &&
    field.json_type === other.json_type &&
    field.array === other.array &&
    field.unit === other.unit &&
    JSON.stringify(field.enum?.map(({ value, name }) => [value, name.en])) ===
      JSON.stringify(other.enum?.map(({ value, name }) => [value, name.en]))
  );
}

export function MixedRuleEditor({
  mapping,
  copyRequested,
  readOnly,
  canWrite,
  onClose,
  onSave,
  onCopy,
  onEdit,
}: {
  mapping?: ApiMappingDetail;
  copyRequested: boolean;
  readOnly: boolean;
  canWrite: boolean;
  onClose: () => void;
  onSave: (saved: ApiMappingDetail) => void;
  onCopy: () => void;
  onEdit: () => void;
}) {
  const t = useTranslations("MixedRule");
  const mt = useTranslations("Mappings");
  const shared = useTranslations("Ingest");
  const wizard = useTranslations("RuleWizard");
  const locale = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();
  const base = mixedDefinition.safeParse(mapping?.definition).data;
  const originalBranches = base?.branches ?? [];
  const initialType = !mapping
    ? ""
    : originalBranches.length === 0
      ? "0"
      : originalBranches.length === 1
        ? String(originalBranches[0].class_uid)
        : "mixed";
  const form = useForm({
    defaultValues: {
      eventType: initialType,
      allowedClasses: [...new Set(originalBranches.map((branch) => branch.class_uid))],
      name: mapping ? (copyRequested ? mt("copyName", { name: mapping.name }) : mapping.name) : "",
      vendor: mapping?.vendor ?? "",
      product: mapping?.product ?? "",
      description: base?.description ?? "",
      mode: base?.parse.mode ?? "json",
      encoding: base?.encoding ?? "",
      pattern: base?.parse.pattern ?? "",
      separator: base?.parse.delimiter?.separator ?? "",
      fieldSeparator: base?.parse.kv?.entry === " " ? "" : (base?.parse.kv?.entry ?? ""),
      keyValueSeparator: base?.parse.kv?.delimiter ?? "",
      quote: base?.parse.kv?.quote ?? "",
      branches: (base?.branches ?? []).map(branchValue),
      mappings: mappingRows(base?.shared),
      processors: base?.processors ?? [],
      samples: [{ value: "" }],
    },
  });
  const data = useWatch({ control: form.control });
  const branches = useWatch({ control: form.control, name: "branches" });
  const rows = useWatch({ control: form.control, name: "mappings" });
  const processors = useWatch({ control: form.control, name: "processors" });
  const samples = useWatch({ control: form.control, name: "samples" });
  const sampleFields = useFieldArray({ control: form.control, name: "samples" });
  const sampleCount = sampleFields.fields.length;
  const [step, setStep] = useState(0);
  const [showRevisions, setShowRevisions] = useState(false);
  const [sampleIndex, setSampleIndex] = useState(0);
  const [selected, setSelected] = useState(0);
  const [fieldError, setFieldError] = useState(false);
  const [submittedRowIDs, setSubmittedRowIDs] = useState<Set<string>>();
  const [typeError, setTypeError] = useState(false);
  const [routingError, setRoutingError] = useState(false);
  const [mappingScope, setMappingScope] = useState("branch");
  const [dismissedMappings, setDismissedMappings] = useState(new Map<string, string[]>());
  const [supplements, setSupplements] = useState<Record<string, unknown>>({});
  const [pendingType, setPendingType] = useState<string>();
  const draftsByType = useRef(
    new Map<string, { branches: BranchValue[]; rows: MappingRow[]; allowedClasses: number[] }>(),
  );
  const eventType = data.eventType ?? initialType;
  const branchTypes = data.allowedClasses ?? [];
  const mixed = eventType === "mixed";
  const showRouting =
    mixed || Boolean(originalBranches.length === 1 && originalBranches[0].match?.length && eventType === initialType);
  const [confirm, setConfirm] = useState(false);
  const [discard, setDiscard] = useState(false);
  const [resumeReview, setResumeReview] = useState(false);
  const focusOnBasics = useRef<RuleTextField | null>(null);
  const focusSample = useRef<number | null>(null);
  const stage = useRef<HTMLElement>(null);
  const hasChanges = form.formState.isDirty;
  const metadataOnly = !readOnly && !copyRequested && !!mapping?.verification && !!base && hasChanges &&
    Object.keys(form.formState.dirtyFields).every((key) => key === "name" || key === "description");
  const branch = branches[selected];
  const branchClass = branch?.class_uid ?? 0;
  const catalog = useQuery({ queryKey: ["ingest", "event-schema"], queryFn: ({ signal }) => listEventClasses(signal) });
  const allFields = useQuery({
    queryKey: ["ingest", "event-schema", "fields"],
    queryFn: ({ signal }) => listEventFields(signal),
  });
  const requestedClasses = [0, ...new Set(branches.map((entry) => entry.class_uid).filter((uid) => uid > 0))];
  const details = useQueries({
    queries: requestedClasses.map((uid) => ({
      queryKey: ["ingest", "event-schema", "class", uid, catalog.data?.catalog_revision],
      queryFn: ({ signal }: { signal: AbortSignal }) => getEventClass(uid, signal),
      enabled: !!catalog.data && step >= 2,
    })),
  });
  const classDetails = (uid: number) => details[requestedClasses.indexOf(uid)]?.data;
  const classFields = classDetails(branchClass);
  const baseClass = classDetails(0);
  const canReadInputs = useCan(permissions.IngestInputRead);
  const inputCounts = useQuery({
    queryKey: ["ingest", "inputs", "counts-by-rule", mapping ? [mapping.id] : []],
    queryFn: ({ signal }) => mappingInputCounts(mapping ? [mapping.id] : [], signal),
    enabled: !!mapping && (!readOnly || showRevisions) && !copyRequested && canReadInputs,
  });
  const boundInputCount = mapping ? (inputCounts.data?.[mapping.id] ?? 0) : 0;
  const fields = allFields.data?.fields ?? [];
  const commonFields = (baseClass?.fields ?? []).filter((field) =>
    requestedClasses.every((uid) =>
      sameFieldMeaning(
        field,
        classDetails(uid)?.fields.find((item) => item.path === field.path),
      ),
    ),
  );
  const selectedDefinition = mappingScope === "shared" ? baseClass : classFields;
  const displayedRows = mappingScope === "shared" || eventType === "0" ? rows : (branch?.fields ?? []);
  const scopedFields: CatalogField[] = (
    mappingScope === "shared" ? commonFields : (selectedDefinition?.fields ?? [])
  ).map((field) => ({
    ...field,
    usages:
      "usages" in field ? (field as CatalogField).usages : [{ class_uid: selectedDefinition?.class_uid ?? 0, field }],
  }));
  const scopedConstraints = mappingScope === "shared" ? [] : (selectedDefinition?.constraints ?? []);
  const scopedConfiguredRows = mappingScope === "branch" && branch ? [...rows, ...displayedRows] : displayedRows;
  const presets = mappingPresets(
    scopedFields,
    scopedConfiguredRows,
    scopedConstraints,
    selectedDefinition?.recommended_mapping_paths ?? [],
  );
  const mappingScopeKey = mappingScope === "shared" ? "shared" : `branch:${branch?.id ?? "base"}:${branchClass}`;
  const classes = (catalog.data?.classes ?? []).map((item) => ({
    uid: item.class_uid,
    name: item.name[locale],
  }));
  const className = (uid: unknown) => classes.find((item) => item.uid === uid)?.name ?? "—";
  const labels = [wizard("basics"), wizard("extraction"), mt("fieldMap"), wizard("review")];
  const submittedSamples = samples.flatMap(({ value }, index) => (value.trim() ? [{ index, value }] : []));
  const inputSamples = submittedSamples.map(({ value }) => value);
  function extractionKey(currentProcessors: typeof processors) {
    return JSON.stringify([
      data.mode,
      data.pattern,
      data.separator,
      data.fieldSeparator,
      data.keyValueSeparator,
      data.quote,
      currentProcessors,
      submittedSamples,
    ]);
  }

  const extractionFingerprint = extractionKey(processors);
  const validationFingerprint = JSON.stringify([
    extractionFingerprint,
    data.encoding,
    data.vendor,
    data.product,
    data.description,
    branches,
    rows,
    catalog.data?.catalog_revision,
  ]);
  const extraction = useRuleRequest<Awaited<ReturnType<typeof extractMapping>>>(extractionFingerprint);
  const validation = useRuleRequest<SampleResult>(validationFingerprint);
  const save = useRuleRequest<ApiMappingDetail>("save");
  const extracted = extraction.data;
  const extractionFailed = extraction.isError;
  const result = validation.data;
  const current = result?.previews?.find((item) => item.index === sampleIndex);
  const extractionItem = extracted?.previews?.find((item) => item.index === sampleIndex);
  const activeFields = { ...supplements, ...(extractionItem?.fields ?? current?.fields ?? {}) };
  const realSources = [
    ...new Set([
      ...(extracted?.previews ?? []).flatMap((item) => Object.keys(item.fields ?? {})),
      ...Object.keys(current?.fields ?? {}),
    ]),
  ];
  const sourceOptions = [...new Set([...realSources, ...Object.keys(supplements)])].sort();
  const sourceTypes = new Map([
    ...Object.entries(supplements).flatMap(([path, value]) => {
      const kind = visibleValueType(value);
      return kind ? [[path, kind] as const] : [];
    }),
    ...(extracted?.previews ?? []).flatMap((item) => Object.entries(item.field_types ?? {})),
    ...Object.entries(extractionItem?.field_types ?? {}),
  ]);
  const matched = branches.find((item) => item.id === current?.branch_id);
  const loadError = [catalog, allFields, ...details].find((query) => query.isError);

  function previewTypeChange(next: string) {
    const current = draftsByType.current.get(next);
    if (current)
      return {
        draft: current,
        removed: 0,
        carried: current.rows.length + current.branches.reduce((count, entry) => count + entry.fields.length, 0),
      };
    const before = [...rows, ...(branches.length === 1 ? branches[0].fields : [])];
    const compatible = before
      .filter((row) => {
        const field = fields.find((item) => item.path === row.target);
        const priorClass = rows.includes(row) ? 0 : branchClass;
        const previous = field?.usages.find((entry) => entry.class_uid === priorClass)?.field;
        const upcoming =
          next === "mixed"
            ? field?.usages.find((entry) => entry.class_uid === 0)?.field
            : field?.usages.find((entry) => entry.class_uid === Number(next))?.field;
        return sameFieldMeaning(previous, upcoming);
      })
      .filter(
        (row, index, candidates) =>
          candidates.findIndex(
            (item) => item.target === row.target || (!item.fixed && !row.fixed && item.source === row.source),
          ) === index,
      );
    const nextBranches =
      next !== "0" && next !== "mixed"
        ? [
            {
              ...branchValue({ ...newBranch("primary"), class_uid: Number(next), name: className(Number(next)) }),
              fields: compatible,
            },
          ]
        : [];
    return {
      draft: { branches: nextBranches, rows: nextBranches.length ? [] : compatible, allowedClasses: [] },
      carried: compatible.length,
      removed: rows.length + branches.reduce((count, entry) => count + entry.fields.length, 0) - compatible.length,
    };
  }

  function selectType(next: string) {
    const { draft } = previewTypeChange(next);
    setPendingType(undefined);
    draftsByType.current.set(eventType, { branches, rows, allowedClasses: branchTypes });
    form.setValue("eventType", next, { shouldDirty: true });
    form.setValue("allowedClasses", draft.allowedClasses, { shouldDirty: true });
    commitRows(draft.rows);
    commitBranches(draft.branches);
    setSelected(0);
    setMappingScope("branch");
    setSubmittedRowIDs(undefined);
    setTypeError(false);
    setRoutingError(false);
  }

  function extractionSettings() {
    const fields = form.getValues();
    const parse: MixedDefinition["parse"] = {
      mode: fields.mode,
      ...(fields.mode === "regex" ? { pattern: fields.pattern } : {}),
      ...(fields.mode === "delimiter"
        ? {
            delimiter: {
              separator: fields.separator,
              columns: base?.parse.delimiter?.columns,
            },
          }
        : {}),
      ...(fields.mode === "kv"
        ? { kv: { delimiter: fields.keyValueSeparator, entry: fields.fieldSeparator, quote: fields.quote } }
        : {}),
    };
    return { parse, processors: fields.processors };
  }

  function definition(): MixedDefinition | undefined {
    const formValues = form.getValues();
    const definitions = sharedMapping(
      formValues.mappings,
      formValues.eventType === "0" ? (baseClass?.fields ?? []) : commonFields,
    );
    const version = catalog.data;
    if (!definitions || version?.contract_version !== "v1") return;
    const compiledBranches: MixedDefinition["branches"] = [];
    for (const entry of formValues.branches) {
      const detail = classDetails(entry.class_uid);
      if (!detail || detail.catalog_revision !== version.catalog_revision) return;
      const mapped = sharedMapping(entry.fields, detail.fields);
      if (!mapped || missingMappingGroups([...formValues.mappings, ...entry.fields], detail.constraints).length) return;
      compiledBranches.push({
        ...mapped,
        id: entry.id,
        name: entry.name.trim(),
        class_uid: entry.class_uid,
        match: entry.conditions.map((row) =>
          row.operation === "equals"
            ? { field: row.field.trim(), equals: row.value }
            : {
                field: row.field.trim(),
                in: row.value
                  .split(",")
                  .map((value) => value.trim())
                  .filter(Boolean),
              },
        ),
      });
    }
    return {
      ...extractionSettings(),
      encoding: formValues.encoding.trim() || undefined,
      contract_version: "v1",
      catalog_revision: version.catalog_revision,
      description: formValues.description,
      shared: definitions,
      branches: compiledBranches,
    };
  }

  async function saveDefinition(value: MixedDefinition) {
    const body = {
      name: form.getValues("name"),
      vendor: data.vendor ?? "",
      product: data.product ?? "",
      definition: { ...value, description: form.getValues("description") },
      samples: inputSamples,
    };
    const saved = await save.run(
      "save",
      (signal) => mapping && !copyRequested
        ? updateMapping(mapping.id, { ...body, expected_version: mapping.version }, signal)
        : createMapping(body, signal),
      (error) => {
        applySampleError(error);
        if (error instanceof APIError && error.fieldErrors.some((item) => item.code === "stale_catalog")) {
          setConfirm(false);
          go(2);
        } else if (applyRuleFieldErrors(error)) {
          setConfirm(false);
          setResumeReview(true);
        }
      },
    );
    if (saved) {
      form.reset(form.getValues());
      onSave(saved);
    }
  }
  useEffect(() => {
    if (step !== 0 || !focusOnBasics.current) return;
    form.setFocus(focusOnBasics.current);
    focusOnBasics.current = null;
  }, [step, form]);
  useEffect(() => {
    if (step !== 0 || focusSample.current === null) return;
    form.setFocus(`samples.${focusSample.current}.value`);
    focusSample.current = null;
  }, [step, form]);
  useEffect(() => {
    if (readOnly || !hasChanges) return;
    function beforeUnload(event: BeforeUnloadEvent) {
      event.preventDefault();
    }
    function handleLink(event: MouseEvent) {
      const anchor = event.target instanceof Element ? event.target.closest<HTMLAnchorElement>("a[href]") : null;
      if (
        anchor &&
        anchor.origin === window.location.origin &&
        anchor.pathname !== window.location.pathname &&
        !event.metaKey &&
        !event.ctrlKey &&
        !window.confirm(t("discard"))
      )
        event.preventDefault();
    }
    window.addEventListener("beforeunload", beforeUnload);
    document.addEventListener("click", handleLink, true);
    return () => {
      window.removeEventListener("beforeunload", beforeUnload);
      document.removeEventListener("click", handleLink, true);
    };
  }, [readOnly, hasChanges, t]);

  function go(next: number, mappingPath?: string) {
    setStep(next);
    setFieldError(false);
    if (focusOnBasics.current) return;
    requestAnimationFrame(() => {
      const field = mappingPath
        ? stage.current?.querySelector<HTMLInputElement>(`[data-mapping-path="${CSS.escape(mappingPath)}"] input`)
        : undefined;
      if (field) field.focus();
      else stage.current?.focus({ preventScroll: true });
    });
  }

  function focusRuleField(field: RuleTextField) {
    if (step === 0) form.setFocus(field);
    else {
      focusOnBasics.current = field;
      go(0);
    }
  }

  function applyRuleFieldErrors(error: unknown) {
    const problems = ruleFieldProblems(error);
    for (const { field, code } of problems) form.setError(field, { type: "server", message: code });
    if (problems.length) focusRuleField(problems[0].field);
    return problems.length > 0;
  }

  function applySampleError(error: unknown) {
    if (!(error instanceof APIError)) return;
    const issue = error.fieldErrors.find(({ field, code }) =>
      /^samples\.\d+$/.test(field) && (code === "too_long" || code === "required"));
    if (!issue) return;
    const index = submittedSamples[Number(issue.field.split(".")[1])]?.index;
    if (index === undefined) return;
    form.setError(`samples.${index}.value`, { type: issue.code }, { shouldFocus: step === 0 });
    if (step !== 0) {
      focusSample.current = index;
      go(0);
    }
  }

  function checkRuleMetadata() {
    const checked = ruleMetadata.safeParse(form.getValues());
    let first: RuleTextField | undefined;
    for (const field of ruleTextFields.options) {
      const issue = checked.error?.issues.find(({ path }) => path[0] === field);
      if (issue) {
        form.setError(field, { type: "validation", message: issue.message });
        first ??= field;
      } else if (form.getFieldState(field).error?.message === "duplicate_name") {
        first ??= field;
      } else {
        form.clearErrors(field);
      }
    }
    if (first) focusRuleField(first);
    return checked.success && !first;
  }

  function ruleRegistration(field: RuleTextField) {
    return form.register(field, {
      validate: (value) => ruleMetadata.shape[field].safeParse(value).error?.issues[0]?.message ?? true,
      onChange: () => {
        if (form.getFieldState(field).error) void form.trigger(field);
      },
    });
  }

  function ruleErrorText(field: RuleTextField) {
    switch (form.formState.errors[field]?.message) {
      case "required":
        return field === "name" ? wizard("nameRequired") : wizard("requiredField");
      case "too_long":
        return wizard("fieldTooLong", { limit: ruleTextLimits[field] });
      case "invalid_characters":
        return field === "description" ? wizard("descriptionInvalidCharacters") : wizard("fieldInvalidCharacters");
      case "duplicate_name":
        return wizard("duplicateName");
      default:
        return undefined;
    }
  }

  async function extract(next?: number) {
    if (readOnly) return;
    if (!inputSamples.length) {
      form.setError("samples.0.value", { type: "required" }, { shouldFocus: true });
      return;
    }
    form.clearErrors("samples");
    const tooLong = submittedSamples.filter(({ value }) => new TextEncoder().encode(value).length > 64 * 1024);
    for (const [position, { index }] of tooLong.entries()) {
      form.setError(`samples.${index}.value`, { type: "too_long" }, { shouldFocus: position === 0 });
    }
    if (tooLong.length) return;
    const settings = extractionSettings();
    if (next !== undefined && settings.parse.mode === "json") {
      const invalid = submittedSamples.filter(({ value }) => !isJSONLogSample(value));
      invalid.forEach(({ index }, position) => {
        form.setError(`samples.${index}.value`, { type: "invalid_json" }, { shouldFocus: position === 0 });
      });
      if (invalid.length) return;
    }
    if (next !== undefined) setSampleIndex(0);
    const key = extractionKey(settings.processors);
    const result = await extraction.run(key, (signal) => extractMapping(settings, inputSamples, signal), applySampleError);
    if (next === undefined || !result?.compiled) return;
    const mismatch = result.previews?.find((item) => item.reason);
    if (mismatch) {
      const sample = submittedSamples[mismatch.index];
      if (sample) form.setFocus(`samples.${sample.index}.value`);
      return;
    }
    if (result.previews?.some((item) => item.fields)) go(next);
  }

  async function runValidation() {
    if (readOnly || !checkRuleMetadata()) return;
    setSubmittedRowIDs(new Set([...rows, ...branches.flatMap((entry) => entry.fields)].map((row) => row.id)));
    const value = definition();
    if (!value) {
      const failedBranch = branches.findIndex((entry) => {
        const detail = classDetails(entry.class_uid);
        return (
          detail &&
          (!sharedMapping(entry.fields, detail.fields) ||
            missingMappingGroups([...rows, ...entry.fields], detail.constraints).length > 0)
        );
      });
      if (failedBranch >= 0) {
        setSelected(failedBranch);
        setMappingScope("branch");
      }
      setFieldError(true);
      requestAnimationFrame(() => {
        const invalid = stage.current?.querySelector<HTMLElement>('[aria-invalid="true"]');
        invalid?.focus();
      });
      return;
    }
    setFieldError(false);
    const result = await validation.run(
      validationFingerprint,
      (signal) => validateMapping(value, inputSamples, {
        vendor: data.vendor ?? "",
        product: data.product ?? "",
        ...((mapping && !copyRequested) ? { parsing_rule_id: mapping.id, expected_version: mapping.version } : {}),
      }, signal),
      (error) => {
        applyRuleFieldErrors(error);
        applySampleError(error);
      },
    );
    if (result?.compiled && result.previews?.length) go(3);
  }

  function commitBranches(next: BranchValue[]) {
    form.setValue("branches", next, { shouldDirty: true });
    setRoutingError(false);
    setFieldError(false);
  }

  function move(delta: number) {
    const next = [...branches];
    const target = selected + delta;
    if (target < 0 || target >= next.length) return;
    [next[selected], next[target]] = [next[target], next[selected]];
    commitBranches(next);
    setSelected(target);
  }

  function commitRows(next: MappingRow[]) {
    form.setValue("mappings", next, { shouldDirty: true });
    setFieldError(false);
  }

  function persist() {
    if (readOnly) return;
    if (!checkRuleMetadata()) {
      setConfirm(false);
      setResumeReview(true);
      return;
    }
    const value = definition();
    if (value) void saveDefinition(value);
  }

  const samplePreviews = (
    <FieldGroup className="gap-3">
      {submittedSamples.map(({ index, value }, position) => {
        const id = `preview-sample-${index}`;
        const label = submittedSamples.length === 1 ? mt("samples") : mt("sampleNumber", { number: index + 1 });
        return (
          <Field key={id} className="gap-1.5">
            {submittedSamples.length > 1 && <FieldLabel htmlFor={id}>{label}</FieldLabel>}
            <Textarea
              id={id}
              aria-label={label}
              variant="three-lines"
              className="bg-muted/40 font-mono data-active:border-primary"
              data-active={(submittedSamples.length > 1 && position === sampleIndex) || undefined}
              readOnly
              spellCheck={false}
              value={value}
              onFocus={() => setSampleIndex(position)}
            />
          </Field>
        );
      })}
    </FieldGroup>
  );
  const visibleSamples = inputSamples;
  const busy = extraction.isPending || validation.isPending;
  const canOpenMapping = readOnly || Boolean(extracted?.previews?.some((item) => item.fields));
  const canPreview =
    readOnly ||
    (!!catalog.data && !!allFields.data && details.every((query) => query.isSuccess) && inputSamples.length > 0);
  const nextAvailable = step === 0 || (step === 1 && canOpenMapping) || (step === 2 && canPreview);

  function navigate(next: number) {
    if (readOnly || next < step) {
      go(next);
    } else if (next === 1) {
      if (!checkRuleMetadata()) return;
      if (!eventType || eventType === "0" || (mixed && branchTypes.length < 2 && !branches.length)) {
        setTypeError(true);
        stage.current?.querySelector<HTMLElement>("#mixed-rule-event-type")?.focus();
        return;
      }
      void extract(1);
    } else if (next === 2) {
      if (
        mixed &&
        (!branches.length ||
          branchTypes.some((uid) => !branches.some((entry) => entry.class_uid === uid)) ||
          branches.some(
            (entry, index) =>
              !entry.name.trim() ||
              !branchTypes.includes(entry.class_uid) ||
              (entry.conditions.length === 0 && index < branches.length - 1) ||
              entry.conditions.some((row) => !row.field.trim() || !row.value.trim()),
          ))
      ) {
        setRoutingError(true);
        return;
      }
      go(2);
    } else if (next === 3) {
      void runValidation();
    }
  }

  const valid = result?.compiled && !!result.previews?.length && !result.failures?.length;
  const badSamples =
    result?.previews?.filter((item) => item.status !== "ok" || (eventType !== "0" && item.event.class_uid === 0))
      .length ?? 0;
  const uncoveredBranches = mixed
    ? branches.filter((entry) => result?.required_branches?.includes(entry.id) &&
        !result.previews?.some((item) => item.branch_id === entry.id && item.status === "ok"))
    : [];
  const hasValidSample = result?.previews?.some((item) => item.status === "ok");
  const reviewRows = [
    ...new Map(
      [...rows, ...(matched?.fields ?? [])].filter((row) => row.target).map((row) => [row.target, row]),
    ).values(),
  ];
  const reviewClass = matched?.class_uid ?? current?.event.class_uid;
  const reviewFields = typeof reviewClass === "number" ? classDetails(reviewClass)?.fields : undefined;
  const staleCatalog =
    result?.compile_issue?.code === "stale_catalog" ||
    (save.error instanceof APIError && save.error.fieldErrors.some((item) => item.code === "stale_catalog"));
  let error: string | undefined;
  if (staleCatalog) error = wizard("staleCatalog");
  else if (loadError) error = mt("schemaUnavailable");
  else if (fieldError) error = wizard("invalidMappings");
  else if (save.error instanceof APIError && save.error.fieldErrors.some(({ code }) => code === "insufficient_coverage"))
    error = wizard("insufficientCoverage");
  else if (step <= 1) {
    if (extractionFailed) error = wizard("extractionFailed");
    else if (extracted?.compile_error) error = extracted.compile_error;
    else if (
      (readOnly || step === 1) &&
      extracted?.previews?.length &&
      extracted.previews.every((item) => !!item.reason)
    )
      error = wizard("noExtracted");
  } else if (validation.isError && !ruleFieldProblems(validation.error).length)
    error = shared("operationFailed");
  else if (result?.compile_error) error = result.compile_error;
  else if (result?.compile_issue)
    error = wizard(
      result.compile_issue.code === "incompatible_shared_field" ? "incompatibleShared" : "requiredMapping",
      {
        field: result.compile_issue.path ?? "",
      },
    );
  else if (save.isError && !ruleFieldProblems(save.error).length)
    error =
      save.error instanceof APIError &&
      save.error.fieldErrors.some(({ field, code }) => field === "expected_version" && code === "revision_conflict")
        ? t("revisionConflict")
        : shared("operationFailed");

  const typeChange = pendingType ? previewTypeChange(pendingType) : undefined;
  const verification = mapping?.verification;
  const sampleVerification = verification ? (
    <div className="flex flex-col gap-1 text-sm">
      <span>{wizard("verifiedSamples", { passed: verification.passed, total: verification.sample_count })}</span>
      <time dateTime={verification.verified_at} className="text-muted-foreground">
        {format.dateTime(new Date(verification.verified_at), { dateStyle: "medium", timeStyle: "short" })}
      </time>
    </div>
  ) : (
    <p className="text-sm text-muted-foreground">{wizard("noVerification")}</p>
  );
  let sampleSidebar = sampleVerification;
  if (!readOnly) {
    sampleSidebar = submittedSamples.length ? samplePreviews : (
      <p className="text-muted-foreground text-sm">{wizard("noSample")}</p>
    );
  }
  const samplePicker = visibleSamples.length > 1 && (
    <Select value={String(sampleIndex)} onValueChange={(value) => value !== null && setSampleIndex(Number(value))}>
      <SelectTrigger aria-label={mt("selectedSample")} className="w-fit max-w-full">
        <SelectValue>{mt("sampleNumber", { number: sampleIndex + 1 })}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {visibleSamples.map((_, index) => {
            const status = result?.previews?.find((item) => item.index === index)?.status;
            return (
              <SelectItem key={index} value={String(index)}>
                {mt("sampleNumber", { number: index + 1 })}
                {status ? ` · ${t(status)}` : ""}
              </SelectItem>
            );
          })}
        </SelectGroup>
      </SelectContent>
    </Select>
  );

  const unmappedIssues = current?.issues?.filter(({ path }) => !reviewRows.some((row) => row.target === path)) ?? [];
  const editReviewedMapping = (path?: string) => {
    if (matched && path && matched.fields.some((row) => row.target === path)) {
      setSelected(branches.indexOf(matched));
      setMappingScope("branch");
    } else if (path && rows.some((row) => row.target === path)) {
      setMappingScope("shared");
    }
    go(2, path);
  };

  return (
    <section className="@container flex min-h-[min(70dvh,42rem)] min-w-0 flex-col gap-5">
      <PageHeader
        title={mapping && !copyRequested ? mapping.name : mt("createMapping")}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => (hasChanges && !readOnly ? setDiscard(true) : onClose())}
            >
              <ArrowLeftIcon data-icon="inline-start" />
              {shared("backToList")}
            </Button>
            {readOnly && mapping && (
              <Button size="sm" variant="outline" onClick={() => setShowRevisions(true)}>
                {wizard("revisionHistory")}
              </Button>
            )}
            {readOnly && canWrite && mapping?.builtin && (
              <Button size="sm" onClick={onCopy}>
                {mt("copyRule")}
              </Button>
            )}
            {readOnly && canWrite && mapping && !mapping.builtin && (
              <Button size="sm" onClick={onEdit}>
                {mt("edit")}
              </Button>
            )}
          </div>
        }
      />
      <WizardProgress
        step={step}
        labels={labels}
        title={mt("title")}
        onNavigate={busy || save.isPending ? undefined : navigate}
        reachable={readOnly ? 3 : step + Number(nextAvailable)}
      />
      {error && (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center gap-2">
            {error}
            {(loadError || staleCatalog) && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  save.reset();
                  validation.reset();
                  void queryClient.invalidateQueries({ queryKey: ["ingest", "event-schema"] });
                }}
              >
                {shared("retry")}
              </Button>
            )}
          </AlertDescription>
        </Alert>
      )}
      <Card className="min-w-0 gap-0 overflow-visible py-0">
        <section
          ref={stage}
          tabIndex={-1}
          aria-label={labels[step]}
          data-readonly={readOnly || undefined}
          className="min-w-0 p-4 focus:outline-none sm:p-6 data-readonly:[&_[data-slot=input-group]:has(:disabled)]:opacity-100 data-readonly:[&_[data-slot=input-group-control]:disabled]:opacity-100 data-readonly:[&_[data-slot=select-trigger]:disabled]:opacity-100"
        >
          {step === 0 && (
            <div className="grid min-w-0 items-start gap-6 @min-[52rem]:grid-cols-[minmax(18rem,0.82fr)_minmax(0,1.18fr)]">
              <IngestSection title={wizard("ruleInfo")}>
                <FieldGroup>
                  <FormInput
                    label={mt("name")}
                    required
                    readOnly={readOnly}
                    registration={ruleRegistration("name")}
                    error={form.formState.errors.name}
                    errorText={ruleErrorText("name")}
                  />
                  <SourceIdentityFields
                    required
                    readOnly={readOnly}
                    vendor={ruleRegistration("vendor")}
                    product={ruleRegistration("product")}
                    errors={form.formState.errors}
                    errorText={{ vendor: ruleErrorText("vendor"), product: ruleErrorText("product") }}
                  />
                  <FormInput
                    label={mt("description")}
                    required
                    readOnly={readOnly}
                    type="textarea"
                    textareaVariant="three-lines"
                    registration={ruleRegistration("description")}
                    error={form.formState.errors.description}
                    errorText={ruleErrorText("description")}
                  />
                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field data-invalid={typeError}>
                      <FieldLabel htmlFor="mixed-rule-event-type" required>
                        {wizard("eventType")}
                      </FieldLabel>
                      <RuleEventTypePicker
                        items={catalog.data?.classes ?? []}
                        value={eventType}
                        invalid={typeError}
                        readOnly={readOnly}
                        onChange={(value) => {
                          if (value === eventType) return;
                          if (branches.some((entry) => entry.fields.length || entry.conditions.length))
                            setPendingType(value);
                          else selectType(value);
                        }}
                      />
                      {typeError && (
                        <FieldError id="event-type-error">
                          {wizard(mixed ? "chooseSeveralTypes" : "chooseEventType")}
                        </FieldError>
                      )}
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="mixed-rule-parser-type" required>
                        {mt("parserType")}
                      </FieldLabel>
                      <Select
                        value={data.mode}
                        disabled={readOnly}
                        onValueChange={(value) => {
                          const mode = parserModes.find((item) => item === value);
                          if (mode && mode !== data.mode) {
                            form.setValue("mode", mode, { shouldDirty: true });
                            form.clearErrors("samples");
                          }
                        }}
                      >
                        <SelectTrigger id="mixed-rule-parser-type" aria-required="true" className="w-full">
                          <SelectValue>{data.mode ? mt(data.mode) : undefined}</SelectValue>
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            {parserModes.map((mode) => (
                              <SelectItem key={mode} value={mode}>
                                {mt(mode)}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </Field>
                  </div>
                  {mixed && (
                    <fieldset className="grid gap-2 rounded-md border p-3 sm:grid-cols-2">
                      <legend className="px-1 text-sm font-medium">{wizard("allowedTypes")}</legend>
                      {classes
                        .filter((item) => item.uid > 0)
                        .map((item) => (
                          <label
                            key={item.uid}
                            htmlFor={`allowed-event-${item.uid}`}
                            className="flex items-center gap-2 text-sm"
                          >
                            <Checkbox
                              id={`allowed-event-${item.uid}`}
                              checked={branchTypes.includes(item.uid)}
                              disabled={readOnly || branches.some((entry) => entry.class_uid === item.uid)}
                              onCheckedChange={(checked) => {
                                form.setValue(
                                  "allowedClasses",
                                  checked ? [...branchTypes, item.uid] : branchTypes.filter((uid) => uid !== item.uid),
                                  { shouldDirty: true },
                                );
                                setTypeError(false);
                              }}
                            />
                            {item.name}
                          </label>
                        ))}
                    </fieldset>
                  )}
                  {data.mode === "regex" && (
                    <Field>
                      <FieldLabel htmlFor="mixed-parse-pattern">{mt("regex")}</FieldLabel>
                      <Textarea
                        id="mixed-parse-pattern"
                        variant="three-lines"
                        placeholder="(?P<message>.+)"
                        className="font-mono"
                        spellCheck={false}
                        readOnly={readOnly}
                        {...form.register("pattern")}
                      />
                    </Field>
                  )}
                  {data.mode === "delimiter" && (
                    <Field>
                      <FieldLabel htmlFor="mixed-parse-separator">{mt("separator")}</FieldLabel>
                      <Input
                        id="mixed-parse-separator"
                        placeholder={mt("separatorPlaceholder")}
                        readOnly={readOnly}
                        {...form.register("separator")}
                      />
                    </Field>
                  )}
                  {data.mode === "kv" && (
                    <div className="grid gap-3 sm:grid-cols-3">
                      {(["fieldSeparator", "keyValueSeparator", "quote"] as const).map((field) => (
                        <Field key={field}>
                          <FieldLabel htmlFor={`mixed-${field}`}>{mt(field)}</FieldLabel>
                          <Input
                            id={`mixed-${field}`}
                            placeholder={mt(`${field}Placeholder`)}
                            readOnly={readOnly}
                            {...form.register(field)}
                          />
                        </Field>
                      ))}
                    </div>
                  )}
                </FieldGroup>
              </IngestSection>
              <IngestSection
                title={readOnly ? wizard("verificationSummary") : mt("samples")}
                required={!readOnly}
                action={
                  !readOnly && (
                    <output className="text-muted-foreground text-sm tabular-nums">
                      {wizard("sampleCount", { count: sampleCount, total: maxRuleSamples })}
                    </output>
                  )
                }
              >
                {readOnly ? sampleVerification : (
                  <>
                    <FieldGroup className="gap-3">
                      {sampleFields.fields.map((field, index) => {
                        const fieldError = form.formState.errors.samples?.[index]?.value;
                        const requestIndex = submittedSamples.findIndex((sample) => sample.index === index);
                        const reason = extracted?.previews?.find((item) => item.index === requestIndex)?.reason;
                        let message: string | undefined;
                        if (fieldError?.type === "invalid_json") message = wizard("sampleInvalidJSON");
                        else if (fieldError?.type === "too_long") message = wizard("sampleTooLong");
                        else if (fieldError?.type === "required" && !inputSamples.length)
                          message = wizard("sampleRequired");
                        else if (reason === "multiple_records") message = wizard("sampleMultipleRecords");
                        else if (reason && data.mode)
                          message = wizard("sampleFormatMismatch", { label: mt(data.mode) });
                        const errorID = message ? `mixed-rule-sample-${field.id}-error` : undefined;
                        return (
                          <Field
                            key={field.id}
                            orientation="horizontal"
                            className="items-start gap-2"
                            data-invalid={!!message}
                          >
                            <div className="grid min-w-0 flex-1 gap-1.5">
                              <Textarea
                                aria-label={
                                  sampleFields.fields.length === 1
                                    ? mt("samples")
                                    : mt("sampleNumber", { number: index + 1 })
                                }
                                aria-required={sampleFields.fields.length === 1}
                                variant="three-lines"
                                aria-invalid={!!message}
                                aria-describedby={errorID}
                                placeholder={wizard("samplePlaceholder")}
                                className="font-mono"
                                spellCheck={false}
                                {...form.register(`samples.${index}.value`, {
                                  onBlur: () => {
                                    const path = `samples.${index}.value` as const;
                                    if (form.getValues("mode") === "json" && form.getValues(path).trim()) {
                                      void form.trigger(path);
                                    }
                                  },
                                  onChange: () => {
                                    const path = `samples.${index}.value` as const;
                                    if (form.getFieldState(path).error?.type === "invalid_json") void form.trigger(path);
                                  },
                                  validate: {
                                    invalid_json: (value) =>
                                      form.getValues("mode") !== "json" ||
                                      !value.trim() ||
                                      isJSONLogSample(value),
                                  },
                                })}
                              />
                              {message && <FieldError id={errorID}>{message}</FieldError>}
                            </div>
                            {sampleFields.fields.length > 1 && (
                              <Button
                                type="button"
                                size="icon-sm"
                                variant="ghost"
                                aria-label={wizard("removeSample", { number: index + 1 })}
                                onClick={() => {
                                  const neighbor = index < sampleFields.fields.length - 1 ? index + 1 : index - 1;
                                  form.setFocus(`samples.${neighbor}.value`);
                                  sampleFields.remove(index);
                                }}
                              >
                                <Trash2Icon />
                              </Button>
                            )}
                          </Field>
                        );
                      })}
                    </FieldGroup>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="self-start"
                      disabled={sampleCount >= maxRuleSamples}
                      onClick={() => sampleFields.append({ value: "" })}
                    >
                      <PlusIcon data-icon="inline-start" />
                      {wizard("addSample")}
                    </Button>
                  </>
                )}
              </IngestSection>
            </div>
          )}
          {step === 1 && (
            <div className="grid min-w-0 items-start gap-6 @min-[52rem]:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
              <div className="grid min-w-0 gap-5">
                <IngestSection
                  title={wizard("extraction")}
                  action={
                    !readOnly &&
                    extractionFailed && (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={busy || !inputSamples.length}
                        onClick={() => extract()}
                      >
                        {shared("retry")}
                      </Button>
                    )
                  }
                >
                  {extractionItem?.reason && (
                    <Alert>
                      <AlertDescription>{t("issue", { code: extractionItem.reason })}</AlertDescription>
                    </Alert>
                  )}
                  {extractionItem?.truncated && (
                    <p className="text-sm text-muted-foreground">{wizard("previewLimit")}</p>
                  )}
                  {extractionItem?.issues?.map((issue) => (
                    <p key={`${issue.path}:${issue.code}`} className="text-sm text-destructive">
                      {issue.path} · {t("issue", { code: issue.code })}
                    </p>
                  ))}
                  {extraction.isPending ? (
                    <Skeleton className="h-40 w-full" />
                  ) : (
                    <ExtractionFields
                      fields={activeFields}
                      supplements={supplements}
                      sourcePaths={realSources}
                      onSupplement={setSupplements}
                      preview={(next, signal) =>
                        extractMapping(
                          { ...extractionSettings(), processors: next },
                          [inputSamples[sampleIndex]],
                          signal,
                        )
                      }
                      fieldTypes={extractionItem?.field_types}
                      processors={processors}
                      readonly={readOnly}
                      onChange={(next) => {
                        form.setValue("processors", next, { shouldDirty: true });
                        void extract();
                      }}
                    />
                  )}
                </IngestSection>
                {showRouting && (
                  <IngestSection
                    title={wizard(mixed ? "routing" : "matchRule")}
                    action={
                      !readOnly &&
                      mixed && (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={branches.length >= 32}
                          onClick={() => {
                            const next = [...form.getValues("branches"), branchValue(newBranch(crypto.randomUUID()))];
                            commitBranches(next);
                            setSelected(next.length - 1);
                          }}
                        >
                          <PlusIcon data-icon="inline-start" />
                          {t("addBranch")}
                        </Button>
                      )
                    }
                  >
                    {branches.length > 0 && (
                      <ol className="divide-y rounded-lg border" aria-label={t("branches")}>
                        {branches.map((entry, index) => (
                          <li key={entry.id}>
                            <Button
                              type="button"
                              variant="ghost"
                              className="h-auto w-full justify-start gap-2 rounded-none px-3 py-2 text-start aria-pressed:bg-muted"
                              aria-pressed={selected === index}
                              onClick={() => setSelected(index)}
                            >
                              <span className="shrink-0 font-mono text-xs text-muted-foreground">{index + 1}</span>
                              <span className="min-w-0 flex-1 truncate">{entry.name || t("unnamedBranch")}</span>
                              {entry.class_uid && (
                                <span className="shrink-0 text-xs text-muted-foreground">
                                  {className(entry.class_uid)}
                                </span>
                              )}
                            </Button>
                          </li>
                        ))}
                      </ol>
                    )}
                    {!readOnly && mixed && branches.length > 0 && (
                      <div className="flex flex-wrap gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={selected === 0}
                          onClick={() => move(-1)}
                        >
                          <ArrowUpIcon data-icon="inline-start" />
                          {t("moveUp")}
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={selected >= branches.length - 1}
                          onClick={() => move(1)}
                        >
                          <ArrowDownIcon data-icon="inline-start" />
                          {t("moveDown")}
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          disabled={!branches.length}
                          onClick={() => {
                            const next = form.getValues("branches").filter((_, index) => index !== selected);
                            commitBranches(next);
                            setSelected(Math.max(0, Math.min(selected, next.length - 1)));
                          }}
                        >
                          <Trash2Icon data-icon="inline-start" />
                          {t("removeBranch")}
                        </Button>
                      </div>
                    )}
                    {routingError && <FieldError>{wizard("routingIncomplete")}</FieldError>}
                    {branch && (
                      <MixedRuleBranch
                        key={`${branch.id}:${branch.class_uid}`}
                        value={branch}
                        readOnly={readOnly}
                        classes={classes.filter((item) => branchTypes.includes(item.uid))}
                        change={(next) =>
                          commitBranches(
                            form.getValues("branches").map((item, index) => (index === selected ? next : item)),
                          )
                        }
                      />
                    )}
                  </IngestSection>
                )}
              </div>
              <IngestSection title={readOnly ? wizard("verificationSummary") : mt("samples")}>
                {sampleSidebar}
                <Collapsible>
                  <CollapsibleTrigger asChild>
                    <Button type="button" variant="ghost" size="sm" className="group/encoding -ml-2">
                      <ChevronDownIcon className="group-aria-expanded/encoding:rotate-180" data-icon="inline-start" />
                      {wizard("advancedSettings")}
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent className="pt-3">
                    <Field>
                      <FieldLabel htmlFor="mixed-rule-encoding">{t("encoding")}</FieldLabel>
                      <Input
                        id="mixed-rule-encoding"
                        placeholder="UTF-8"
                        readOnly={readOnly}
                        aria-describedby="mixed-rule-encoding-help"
                        {...form.register("encoding")}
                      />
                      <FieldDescription id="mixed-rule-encoding-help">{wizard("encodingHint")}</FieldDescription>
                    </Field>
                  </CollapsibleContent>
                </Collapsible>
              </IngestSection>
            </div>
          )}
          {step === 2 && (
            <div className="grid min-w-0 items-start gap-6 @min-[52rem]:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
              <div className="grid min-w-0 gap-5">
                <IngestSection
                  title={mt("fieldMap")}
                  action={
                    mixed || (branches.length > 0 && rows.length) ? (
                      <Select
                        value={mappingScope === "shared" ? "shared" : String(selected)}
                        onValueChange={(value) => {
                          if (!value) return;
                          setMappingScope(value === "shared" ? "shared" : "branch");
                          if (value !== "shared") setSelected(Number(value));
                        }}
                      >
                        <SelectTrigger aria-label={wizard("mappingScope")} className="w-fit max-w-full">
                          <SelectValue>
                            {mappingScope === "shared"
                              ? wizard("commonFields")
                              : mixed
                                ? branch?.name
                                : className(branchClass)}
                          </SelectValue>
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            {branches.map((entry, index) => (
                              <SelectItem key={entry.id} value={String(index)}>
                                {mixed ? `${entry.name} · ${className(entry.class_uid)}` : className(entry.class_uid)}
                              </SelectItem>
                            ))}
                            <SelectItem value="shared">{wizard("commonFields")}</SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    ) : (
                      <Badge variant="secondary">{className(branchClass)}</Badge>
                    )
                  }
                >
                  {!selectedDefinition ? (
                    <Skeleton className="h-28 w-full" />
                  ) : (
                    <RuleMappingRows
                      key={mappingScopeKey}
                      rows={displayedRows}
                      fields={scopedFields}
                      classes={classes}
                      presets={presets}
                      dismissed={dismissedMappings.get(mappingScopeKey)}
                      onDismiss={(path) =>
                        setDismissedMappings((current) =>
                          new Map(current).set(mappingScopeKey, [...(current.get(mappingScopeKey) ?? []), path]),
                        )
                      }
                      missing={submittedRowIDs ? missingMappingGroups(scopedConfiguredRows, scopedConstraints) : []}
                      sources={sourceOptions}
                      sourceTypes={sourceTypes}
                      submittedRowIDs={submittedRowIDs}
                      readOnly={readOnly}
                      onChange={(next) => {
                        if (mappingScope === "shared" || eventType === "0") commitRows(next);
                        else
                          commitBranches(
                            branches.map((entry, index) => (index === selected ? { ...entry, fields: next } : entry)),
                          );
                        setFieldError(false);
                      }}
                    />
                  )}
                </IngestSection>
              </div>
              <IngestSection title={readOnly ? wizard("verificationSummary") : mt("samples")}>
                {sampleSidebar}
              </IngestSection>
            </div>
          )}
          {step === 3 && (
            <div className="grid min-w-0 items-start gap-6 @min-[52rem]:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
              <IngestSection title={mt("preview")} action={<div className="@min-[52rem]:hidden">{samplePicker}</div>}>
                {current ? (
                  <div className="grid min-w-0 gap-4">
                    <div className="flex flex-wrap items-center gap-2" aria-live="polite">
                      <span className="text-base font-semibold">
                        {current.event.class_uid === 0 ? wizard("unclassified") : className(current.event.class_uid)}
                      </span>
                      <Badge variant={current.status === "error" ? "destructive" : "secondary"}>
                        {t(current.status)}
                      </Badge>
                      {mixed && matched && <span className="text-sm text-muted-foreground">{matched.name}</span>}
                      {current.candidate_class_uid && current.event.class_uid !== current.candidate_class_uid && (
                        <span className="text-sm text-muted-foreground">
                          {t("candidate", { name: className(current.candidate_class_uid) })}
                        </span>
                      )}
                    </div>
                    {!!uncoveredBranches.length && (
                      <p className="text-sm text-muted-foreground">
                        {wizard("uncoveredBranches", {
                          names: uncoveredBranches.map((entry) => entry.name).join(", "),
                        })}
                      </p>
                    )}
                    {unmappedIssues.length > 0 && (
                      <Alert variant="destructive">
                        <AlertDescription>
                          {unmappedIssues.map((issue, index) => (
                            <div key={`${issue.code}:${index}`}>
                              {t("issue", { code: issue.code })}
                              {issue.path &&
                                ` · ${reviewFields?.find((field) => field.path === issue.path)?.name[locale] ?? issue.path}`}
                            </div>
                          ))}
                          {!readOnly && (
                            <Button
                              variant="link"
                              size="sm"
                              className="h-auto px-0"
                              onClick={() => editReviewedMapping(unmappedIssues[0]?.path)}
                            >
                              {wizard("editMapping")}
                            </Button>
                          )}
                        </AlertDescription>
                      </Alert>
                    )}
                    <Tabs defaultValue="fields" className="min-w-0">
                      <TabsList variant="line" className="w-full justify-start border-b">
                        <TabsTrigger value="fields" className="flex-none">
                          {wizard("mappedOutput")}
                        </TabsTrigger>
                        <TabsTrigger value="json" className="flex-none">
                          OCSF JSON
                        </TabsTrigger>
                      </TabsList>
                      <TabsContent value="fields" className="min-w-0 pt-2">
                        {reviewRows.length ? (
                          <div className="overflow-hidden rounded-md border">
                            <Table className="table-fixed">
                              <TableHeader>
                                <TableRow>
                                  <TableHead className="w-2/5">{wizard("standardField")}</TableHead>
                                  <TableHead className="w-1/4">{mt("sourceField")}</TableHead>
                                  <TableHead>{t("normalized")}</TableHead>
                                </TableRow>
                              </TableHeader>
                              <TableBody>
                                {reviewRows.map((row) => {
                                  const field = reviewFields?.find((item) => item.path === row.target);
                                  const value = readEventValue(current.event, row.target);
                                  const label = field?.enum?.find((entry) => String(entry.value) === value)?.name[
                                    locale
                                  ];
                                  const issues = current.issues?.filter(({ path }) => path === row.target) ?? [];
                                  return (
                                    <TableRow key={row.target} className="align-top">
                                      <TableCell className="whitespace-normal break-words font-medium">
                                        {field?.name[locale] ?? row.target}
                                        {field && (
                                          <code className="mt-1 block break-all text-xs font-normal text-muted-foreground">
                                            {row.target}
                                          </code>
                                        )}
                                      </TableCell>
                                      <TableCell className="whitespace-normal break-all font-mono text-xs">
                                        {row.source}
                                      </TableCell>
                                      <TableCell className="whitespace-normal break-words">
                                        <div className="flex flex-wrap items-center gap-1.5">
                                          {label ?? (
                                            <code className="break-all">
                                              {value === "[REDACTED]" ? wizard("hidden") : value}
                                            </code>
                                          )}
                                          {current.applied_defaults?.includes(row.target) && (
                                            <Badge variant="outline">{wizard("defaultValue")}</Badge>
                                          )}
                                        </div>
                                        {issues.map((issue) => (
                                          <div key={issue.code} className="mt-1 text-xs text-destructive">
                                            {t("issue", { code: issue.code })}
                                          </div>
                                        ))}
                                        {!!issues.length && !readOnly && (
                                          <Button
                                            size="sm"
                                            variant="link"
                                            className="h-auto px-0 text-xs"
                                            onClick={() => editReviewedMapping(row.target)}
                                          >
                                            {wizard("editMapping")}
                                          </Button>
                                        )}
                                      </TableCell>
                                    </TableRow>
                                  );
                                })}
                              </TableBody>
                            </Table>
                          </div>
                        ) : (
                          <p className="text-sm text-muted-foreground">{wizard("noMappedOutput")}</p>
                        )}
                      </TabsContent>
                      <TabsContent value="json" className="min-w-0 pt-2">
                        <pre className="max-h-96 overflow-auto rounded-md border bg-muted/40 p-3 text-xs">
                          {JSON.stringify(current.event, null, 2)}
                        </pre>
                      </TabsContent>
                    </Tabs>
                    {!readOnly && boundInputCount > 0 && (
                      <p className="text-sm text-muted-foreground">
                        {wizard("dataInputImpact", { count: boundInputCount })}
                      </p>
                    )}
                  </div>
                ) : readOnly ? (
                  sampleVerification
                ) : (
                  <p className="text-sm text-muted-foreground">{t("stale")}</p>
                )}
              </IngestSection>
              {!readOnly && (
                <IngestSection
                  title={mt("samples")}
                  action={<div className="hidden @min-[52rem]:block">{samplePicker}</div>}
                >
                  {visibleSamples[sampleIndex] && (
                    <Textarea
                      aria-label={mt("samples")}
                      readOnly
                      spellCheck={false}
                      variant="three-lines"
                      className="bg-muted/40 font-mono"
                      value={visibleSamples[sampleIndex]}
                    />
                  )}
                </IngestSection>
              )}
            </div>
          )}
        </section>
        <div className="sticky bottom-0 z-10 mt-auto flex flex-wrap justify-end gap-2 rounded-b-xl border-t bg-card px-4 py-3 sm:px-6">
          {step > 0 && (
            <Button size="sm" variant="outline" disabled={busy || save.isPending} onClick={() => go(step - 1)}>
              {wizard("previous")}
            </Button>
          )}
          {step === 0 && resumeReview && valid && (
            <Button size="sm" onClick={() => checkRuleMetadata() && go(3)}>
              {wizard("review")}
            </Button>
          )}
          {step === 0 && (!resumeReview || !valid) && (
            <Button size="sm" disabled={busy || (!inputSamples.length && !readOnly)} onClick={() => navigate(1)}>
              {busy && <Spinner data-icon="inline-start" />}
              {wizard("extraction")}
            </Button>
          )}
          {step === 0 && metadataOnly && base && (
            <Button
              size="sm"
              disabled={busy || save.isPending}
              onClick={() => {
                if (checkRuleMetadata()) void saveDefinition(base);
              }}
            >
              {mt("save")}
            </Button>
          )}
          {step === 1 && (
            <Button size="sm" disabled={busy || !canOpenMapping} onClick={() => navigate(2)}>
              {mt("fieldMap")}
            </Button>
          )}
          {step === 2 && (
            <Button size="sm" disabled={busy || !canPreview} onClick={() => navigate(3)}>
              {busy && <Spinner data-icon="inline-start" />}
              {wizard(readOnly ? "review" : "validateOutput")}
            </Button>
          )}
          {step === 3 && !readOnly && (
            <>
              {!valid && (
                <Button size="sm" variant="outline" disabled={busy} onClick={runValidation}>
                  {wizard("validateOutput")}
                </Button>
              )}
              <Button
                size="sm"
                disabled={!valid || !hasValidSample || uncoveredBranches.length > 0 || save.isPending}
                onClick={() => {
                  if (badSamples) setConfirm(true);
                  else persist();
                }}
              >
                {mapping && !copyRequested ? mt("save") : mt("create")}
              </Button>
            </>
          )}
        </div>
      </Card>
      {pendingType && typeChange && (
        <IngestConfirm
          title={wizard("changeEventType")}
          description={wizard("changeEventTypeHint", { carried: typeChange.carried, removed: typeChange.removed })}
          action={wizard("changeEventType")}
          pending={false}
          error={false}
          onCancel={() => setPendingType(undefined)}
          onConfirm={() => selectType(pendingType)}
        />
      )}
      {confirm && (
        <IngestConfirm
          title={wizard("confirmSave")}
          description={wizard("sampleImpact", { count: badSamples })}
          action={mt("save")}
          pending={save.isPending}
          error={save.isError}
          onCancel={() => setConfirm(false)}
          onConfirm={persist}
        />
      )}
      {showRevisions && mapping && (
        <RuleRevisions
          mapping={mapping}
          canRestore={canWrite && !mapping.builtin && inputCounts.isSuccess}
          inputCount={boundInputCount}
          onClose={() => setShowRevisions(false)}
          onRestore={(saved) => {
            setShowRevisions(false);
            onSave(saved);
          }}
        />
      )}
      {discard && (
        <IngestConfirm
          title={t("discard")}
          description={wizard("discardDescription")}
          action={wizard("discardAction")}
          pending={false}
          error={false}
          onCancel={() => setDiscard(false)}
          onConfirm={onClose}
        />
      )}
    </section>
  );
}
