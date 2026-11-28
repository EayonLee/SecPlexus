"use client";

import { useId, useMemo, useState } from "react";

import { useRouter } from "next/navigation";

import { useTranslations } from "next-intl";
import { toast } from "sonner";

import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import { Field, FieldLabel } from "@/components/ui/field";
import { updateProfile } from "@/spx/api/iam/auth";
import { zApiProfileUpdateBody } from "@/spx/api/iam/schemas";

import { FormDialog } from "../components/forms";
import { refreshMe, useIdentity } from "../iam/session";

const pageSize = 60;

export function ProfileTimeZoneDialog({ onClose, returnFocus }: { onClose: () => void; returnFocus: HTMLElement }) {
  const t = useTranslations("Account");
  const management = useTranslations("Management");
  const id = useId();
  const router = useRouter();
  const stored = useIdentity((state) => state.timezone) || "browser";
  const [zone, setZone] = useState(stored);
  const [query, setQuery] = useState("");
  const [visible, setVisible] = useState(pageSize);
  const zones = useMemo(() => Intl.supportedValuesOf?.("timeZone") ?? [], []);
  const needle = query.trim().toLowerCase().replace(/_/g, " ");
  const matches = zones.filter((candidate) => candidate.toLowerCase().replace(/_/g, " ").includes(needle));
  const candidates = matches.slice(0, visible);
  const label = (candidate: string) => {
    if (candidate === "utc") return t("timeZoneUtc");
    if (candidate === "browser") return t("timeZoneBrowser");
    return candidate;
  };
  const specialZones = ["browser", "utc"].filter(
    (candidate) => candidate.includes(needle) || label(candidate).toLowerCase().includes(needle),
  );
  const items = [...specialZones, ...candidates];

  return (
    <FormDialog
      title={t("changeTimeZone")}
      layout="product"
      returnFocus={returnFocus}
      schema={zApiProfileUpdateBody}
      defaultValues={{ timezone: stored }}
      submitLabel={management("save")}
      submitDisabled={zone === stored}
      onClose={onClose}
      onSubmit={async ({ timezone }) => {
        await updateProfile({ timezone });
        await refreshMe();
        router.refresh();
        toast.success(t("timeZoneSaved"));
      }}
    >
      {(form) => (
        <Field>
          <FieldLabel htmlFor={id}>{t("timeZone")}</FieldLabel>
          <Combobox
            items={items}
            filteredItems={items}
            filter={null}
            value={zone}
            itemToStringLabel={label}
            onValueChange={(next) => {
              if (!next) return;
              setZone(next);
              form.setValue("timezone", next);
            }}
            onInputValueChange={(value, event) => {
              if (event.reason !== "input-change" && event.reason !== "input-clear") return;
              setQuery(value);
              setVisible(pageSize);
            }}
            disabled={form.formState.isSubmitting}
          >
            <ComboboxInput id={id} aria-label={t("timeZone")} placeholder={label(zone)} className="w-full" />
            <ComboboxContent>
              <ComboboxList
                onScroll={(event) => {
                  const list = event.currentTarget;
                  if (visible < matches.length && list.scrollHeight - list.scrollTop - list.clientHeight < 48) {
                    setVisible((count) => count + pageSize);
                  }
                }}
              >
                {items.map((candidate) => (
                  <ComboboxItem key={candidate} value={candidate}>
                    {label(candidate)}
                  </ComboboxItem>
                ))}
              </ComboboxList>
              <ComboboxEmpty>{t("timeZoneEmpty")}</ComboboxEmpty>
            </ComboboxContent>
          </Combobox>
        </Field>
      )}
    </FormDialog>
  );
}
