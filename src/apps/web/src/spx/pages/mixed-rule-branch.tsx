"use client";

import { PlusIcon, Trash2Icon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

import type { RuleBranch } from "./mixed-rule-model";
import { type MappingRow, mappingRows } from "./rule-mapping-rows";

export type BranchValue = Pick<RuleBranch, "id" | "name" | "class_uid"> & {
  fields: MappingRow[];
  conditions: { id: string; field: string; operation: "equals" | "in"; value: string }[];
};

export function branchValue(branch: RuleBranch): BranchValue {
  return {
    id: branch.id,
    name: branch.name,
    class_uid: branch.class_uid,
    fields: mappingRows(branch),
    conditions: (branch.match ?? []).map((item) => ({
      id: item.field,
      field: item.field,
      operation: "equals" in item ? "equals" : "in",
      value: "equals" in item ? item.equals : item.in.join(", "),
    })),
  };
}

export function MixedRuleBranch({
  value,
  change,
  readOnly,
  classes,
}: {
  value: BranchValue;
  change: (next: BranchValue) => void;
  readOnly: boolean;
  classes: { uid: number; name: string }[];
}) {
  const t = useTranslations("MixedRule");
  return (
    <FieldGroup className="gap-5">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="branch-name">{t("branchName")}</FieldLabel>
          <Input
            id="branch-name"
            value={value.name}
            readOnly={readOnly}
            onChange={(event) => change({ ...value, name: event.target.value })}
            maxLength={160}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="branch-type">{t("eventType")}</FieldLabel>
          <Select
            disabled={readOnly}
            value={value.class_uid ? String(value.class_uid) : ""}
            onValueChange={(uid) => {
              if (uid) change({ ...value, class_uid: Number(uid), fields: [] });
            }}
          >
            <SelectTrigger id="branch-type" className="w-full">
              <SelectValue placeholder={t("chooseType")}>
                {classes.find((item) => item.uid === value.class_uid)?.name}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {classes.map((item) => (
                  <SelectItem value={String(item.uid)} key={item.uid}>
                    {item.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      </div>
      <fieldset className="grid min-w-0 gap-2">
        <legend className="mb-2 text-sm font-medium">{t("conditions")}</legend>
        <p className="text-xs text-muted-foreground">
          {t(value.conditions.length ? "allConditions" : "alwaysMatches")}
        </p>
        {value.conditions.map((condition, index) => {
          const replace = (next: typeof condition) =>
            change({ ...value, conditions: value.conditions.map((item) => (item.id === condition.id ? next : item)) });
          return (
            <div
              key={condition.id}
              className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-2 @min-[27rem]/branch:grid-cols-[minmax(0,1fr)_minmax(0,0.8fr)_minmax(0,1fr)_auto]"
            >
              <Input
                aria-label={t("matchField", { number: index + 1 })}
                value={condition.field}
                readOnly={readOnly}
                placeholder="cat"
                onChange={(event) => replace({ ...condition, field: event.target.value })}
              />
              <Select
                value={condition.operation}
                disabled={readOnly}
                onValueChange={(operation) => {
                  if (operation === "in" || operation === "equals") replace({ ...condition, operation });
                }}
              >
                <SelectTrigger aria-label={t("operation", { number: index + 1 })} className="w-full">
                  <SelectValue>{t(condition.operation)}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="equals">{t("equals")}</SelectItem>
                    <SelectItem value="in">{t("in")}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
              <Input
                aria-label={t("matchValue", { number: index + 1 })}
                value={condition.value}
                readOnly={readOnly}
                placeholder={condition.operation === "in" ? "a, b" : "event:system"}
                onChange={(event) => replace({ ...condition, value: event.target.value })}
              />
              {!readOnly && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("removeCondition", { number: index + 1 })}
                  onClick={() =>
                    change({ ...value, conditions: value.conditions.filter((item) => item.id !== condition.id) })
                  }
                >
                  <Trash2Icon />
                </Button>
              )}
            </div>
          );
        })}
        {!readOnly && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="justify-self-start"
            onClick={() =>
              change({
                ...value,
                conditions: [
                  ...value.conditions,
                  { id: crypto.randomUUID(), field: "", operation: "equals", value: "" },
                ],
              })
            }
          >
            <PlusIcon data-icon="inline-start" />
            {t("addCondition")}
          </Button>
        )}
      </fieldset>
    </FieldGroup>
  );
}
