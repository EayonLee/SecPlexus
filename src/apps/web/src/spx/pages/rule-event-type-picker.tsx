"use client";

import { useLocale, useTranslations } from "next-intl";

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
import type { EventClassSummary } from "@/spx/api/ingest/event-schema";

import {
  ListSearchContent,
  ListSearchOption,
  ListSearchTrigger,
  useListSearchState,
} from "../components/list-controls";

type Option = { value: string; label: string; search: string };

export function RuleEventTypePicker({
  items,
  value,
  invalid = false,
  readOnly = false,
  filterMode = false,
  onChange,
}: {
  items: EventClassSummary[];
  value: string;
  invalid?: boolean;
  readOnly?: boolean;
  filterMode?: boolean;
  onChange: (value: string) => void;
}) {
  const locale = useLocale();
  const t = useTranslations("RuleWizard");
  const common = useTranslations("Common");
  const search = useListSearchState();
  const grouped = new Map<number, { label: string; items: Option[] }>();
  for (const item of items) {
    if (item.class_uid === 0) continue;
    const group = grouped.get(item.category_uid) ?? { label: item.category[locale], items: [] };
    group.items.push({
      value: String(item.class_uid),
      label: item.name[locale],
      search: [item.class_uid, item.name.en, item.name["zh-CN"], ...Object.values(item.aliases ?? {}).flat()].join(" "),
    });
    grouped.set(item.category_uid, group);
  }
  const groups = [...grouped.values()];
  if (filterMode) {
    const all = common("allOptions");
    groups.unshift({ label: "", items: [{ value: "", label: all, search: all }] });
  }
  const current = groups.flatMap((group) => group.items).find((item) => item.value === value);
  const legacy =
    value === "mixed"
      ? t("multipleTypes")
      : value === "0"
        ? items.find((item) => item.class_uid === 0)?.name[locale]
        : undefined;
  const selected = current ?? (legacy ? { value, label: legacy, search: "" } : null);
  const results = (
    <>
      <ComboboxEmpty>{t("noEventTypes")}</ComboboxEmpty>
      <ComboboxList>
        {(group: (typeof groups)[number]) => (
          <ComboboxGroup key={group.label} items={group.items}>
            {group.label && <ComboboxLabel>{group.label}</ComboboxLabel>}
            <ComboboxCollection>
              {(item: Option) =>
                filterMode ? (
                  <ListSearchOption key={item.value} value={item} label={item.label} />
                ) : (
                  <ComboboxItem key={item.value} value={item} className="pl-4">
                    {item.label}
                  </ComboboxItem>
                )
              }
            </ComboboxCollection>
          </ComboboxGroup>
        )}
      </ComboboxList>
    </>
  );
  return (
    <Combobox
      items={groups}
      value={selected}
      disabled={readOnly}
      openOnInputClick={!filterMode}
      inputValue={filterMode ? (search.text ?? "") : undefined}
      onOpenChange={filterMode ? search.onOpenChange : undefined}
      onInputValueChange={filterMode ? search.onInputValueChange : undefined}
      isItemEqualToValue={(item, current) => item.value === current.value}
      itemToStringLabel={(item) => item.label}
      filter={(item, query) => item.search.toLowerCase().includes(query.trim().toLowerCase())}
      onValueChange={(item) => item && onChange(item.value)}
    >
      {filterMode ? (
        <>
          <ListSearchTrigger label={t("eventType")} value={selected?.label} />
          <ListSearchContent label={t("eventType")}>{results}</ListSearchContent>
        </>
      ) : (
        <>
          <ComboboxInput
            id="mixed-rule-event-type"
            className="w-full"
            placeholder={t("chooseEventType")}
            disabled={readOnly}
            aria-required="true"
            aria-invalid={invalid}
            aria-describedby={invalid ? "event-type-error" : undefined}
          />
          <ComboboxContent>{results}</ComboboxContent>
        </>
      )}
    </Combobox>
  );
}
