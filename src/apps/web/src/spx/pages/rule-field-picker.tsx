"use client";

import { useEffect, useRef } from "react";

import { InfoIcon } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Combobox,
  ComboboxCollection,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxLabel,
  ComboboxList,
} from "@/components/ui/combobox";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import type { CatalogField } from "@/spx/api/ingest/event-schema";

import { mappingFieldType } from "./rule-catalog";

type FieldGroup = "basic" | "source" | "identity" | "network" | "host" | "web" | "cloud" | "findings";

function groupOf(path: string): FieldGroup {
  const root = path.split(".", 1)[0];
  switch (root) {
    case "metadata":
      return "source";
    case "actor":
    case "user":
    case "service":
      return "identity";
    case "dst_endpoint":
    case "src_endpoint":
    case "connection_info":
    case "traffic":
    case "cumulative_traffic":
      return "network";
    case "device":
    case "process":
    case "file":
      return "host";
    case "http_request":
    case "http_response":
    case "query":
    case "rcode":
      return "web";
    case "api":
    case "cloud":
      return "cloud";
    case "finding_info":
    case "vulnerabilities":
      return "findings";
    default:
      return "basic";
  }
}

export function RuleFieldPicker({
  fields,
  value,
  onChange,
  label,
  classes,
  errorID,
  focusOnMount = false,
}: {
  fields: CatalogField[];
  value: string;
  onChange: (path: string) => void;
  label: string;
  classes: { uid: number; name: string }[];
  errorID?: string;
  focusOnMount?: boolean;
}) {
  const t = useTranslations("RuleWizard");
  const mt = useTranslations("Mappings");
  const locale = useLocale();
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (focusOnMount) input.current?.focus();
  }, [focusOnMount]);
  const available = fields.filter(
    (field) =>
      (field.owner === "mapping" && (field.json_type !== "object" || !!field.object_fields?.length)) ||
      field.path === "time" ||
      field.path === "severity_id",
  );
  const selected = available.find((field) => field.path === value) ?? null;
  const groups = (["basic", "source", "identity", "network", "host", "web", "cloud", "findings"] as const)
    .map((group) => ({ value: group, items: available.filter((field) => groupOf(field.path) === group) }))
    .filter((group) => group.items.length);
  return (
    <div className="flex min-w-0 items-center gap-1">
      <Combobox
        items={groups}
        value={selected}
        isItemEqualToValue={(field, current) => field.path === current.path}
        itemToStringLabel={(field) => field.name[locale]}
        filter={(field, search) =>
          [
            field.path,
            field.name.en,
            field.name["zh-CN"],
            field.description[locale],
            ...field.usages.map((item) => item.field.name[locale]),
            field.path === "dst_endpoint.ip" ? "目的 IP 目的地址 目标地址 destination" : "",
            field.path === "src_endpoint.ip" ? "来源 IP 来源地址 源地址 source" : "",
          ]
            .join(" ")
            .toLowerCase()
            .includes(search.trim().toLowerCase())
        }
        onValueChange={(item) => onChange(item?.path ?? "")}
      >
        <ComboboxInput
          ref={input}
          aria-label={label}
          aria-invalid={!!errorID}
          aria-describedby={errorID}
          placeholder={t("chooseField")}
          className="w-full min-w-0"
        />
        <ComboboxContent className="min-w-[min(26rem,var(--available-width))]">
          <ComboboxEmpty>{mt("noFields")}</ComboboxEmpty>
          <ComboboxList>
            {(group: (typeof groups)[number]) => (
              <ComboboxGroup key={group.value} items={group.items}>
                <ComboboxLabel>{t(group.value)}</ComboboxLabel>
                <ComboboxCollection>
                  {(field: CatalogField) => {
                    const type = mappingFieldType(field);
                    return (
                      <ComboboxItem key={field.path} value={field} className="pl-4" title={field.path}>
                        <span className="grid w-full min-w-0 grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto] items-center gap-2">
                          <span className="truncate">{field.name[locale]}</span>
                          <code className="text-muted-foreground truncate text-xs">{field.path}</code>
                          <Badge variant="secondary" className="value-type-badge" data-value-type={type.kind}>
                            {type.label}
                          </Badge>
                        </span>
                      </ComboboxItem>
                    );
                  }}
                </ComboboxCollection>
              </ComboboxGroup>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
      {selected && (
        <Popover>
          <PopoverTrigger asChild>
            <Button size="icon-sm" variant="ghost" aria-label={mt("fieldDetails")}>
              <InfoIcon />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="flex max-h-80 flex-col gap-2 overflow-auto text-sm">
            <strong>{selected.name[locale]}</strong>
            <code className="break-all text-xs">
              {selected.path} · {selected.logical_type}
            </code>
            <p className="text-muted-foreground">{selected.description[locale]}</p>
            <p className="text-muted-foreground">{mt(selected.array ? "fieldArray" : "fieldScalar")}</p>
            <ul className="grid gap-2">
              {selected.usages.map(({ class_uid, field }) => (
                <li key={class_uid} className="border-t pt-2">
                  <p>{classes.find((item) => item.uid === class_uid)?.name ?? class_uid}</p>
                  <small className="text-muted-foreground">
                    {mt(
                      field.requirement === "required"
                        ? "fieldRequiredLevel"
                        : field.requirement === "recommended"
                          ? "fieldRecommendedLevel"
                          : "fieldOptionalLevel",
                    )}
                    {field.condition?.[locale] && <> · {field.condition[locale]}</>}
                  </small>
                  {field.enum && (
                    <dl className="mt-1 grid gap-1 text-xs">
                      {field.enum.map((entry) => (
                        <div key={entry.value}>
                          <dt>{entry.name[locale]}</dt>
                          {entry.description?.[locale] && (
                            <dd className="text-muted-foreground">{entry.description[locale]}</dd>
                          )}
                        </div>
                      ))}
                    </dl>
                  )}
                </li>
              ))}
            </ul>
          </PopoverContent>
        </Popover>
      )}
    </div>
  );
}
