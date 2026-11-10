"use client";

import { useMemo, useState } from "react";

import { useRouter } from "next/navigation";

import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import { updateProfile } from "@/spx/api/iam/auth";

import { refreshMe, useIdentity } from "../iam/session";

/** 每批渲染的候选数。全部时区都可滚动到达，这里只控制单次挂载的 DOM 规模。 */
const pageSize = 60;

function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

function allTimeZones(): string[] {
  try {
    return Intl.supportedValuesOf("timeZone");
  } catch {
    return [];
  }
}

/** 选择展示时区；写入后端后刷新身份，使 formatter 立即按新时区渲染。 */
export function TimeZoneCard() {
  const t = useTranslations("Account");
  const router = useRouter();
  const auth = useIdentity();
  const [saving, setSaving] = useState(false);
  const stored = auth.timezone ?? "";
  const [value, setValue] = useState(stored);
  const [query, setQuery] = useState("");
  const [visible, setVisible] = useState(pageSize);

  const zones = useMemo(allTimeZones, []);
  const resolved = stored === "" || stored === "browser" ? browserTimeZone() : stored;

  const label = (zone: string) => {
    if (zone === "utc") return t("timeZoneUtc");
    if (zone === "browser") return t("timeZoneBrowser");
    return zone;
  };

  const matches = useMemo(() => {
    const needle = query.trim().toLowerCase().replace(/_/g, " ");
    if (needle === "") return zones;
    return zones.filter((zone) => zone.toLowerCase().replace(/_/g, " ").includes(needle));
  }, [query, zones]);

  const candidates = matches.slice(0, visible);
  const remaining = matches.length - candidates.length;

  async function change(next: string) {
    if (!next) return;
    setValue(next);
    setSaving(true);
    try {
      await updateProfile({ timezone: next });
      await refreshMe();
      router.refresh();
      toast.success(t("timeZoneSaved"));
    } catch {
      setValue(stored);
      toast.error(t("timeZoneFailed"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("timeZone")}</CardTitle>
      </CardHeader>
      <CardContent>
        <Combobox
          items={candidates}
          filteredItems={candidates}
          filter={null}
          value={value === "" ? "browser" : value}
          onValueChange={(next) => void change(next ?? "")}
          onInputValueChange={(text, event) => {
            if (event.reason !== "input-change" && event.reason !== "input-clear") return;
            setQuery(text);
            setVisible(pageSize);
          }}
          itemToStringLabel={label}
          disabled={saving}
        >
          <ComboboxInput aria-label={t("timeZone")} className="w-full sm:w-80" placeholder={resolved} />
          <ComboboxContent>
            <ComboboxList
              onScroll={(event) => {
                const list = event.currentTarget;
                if (remaining > 0 && list.scrollHeight - list.scrollTop - list.clientHeight < 48)
                  setVisible((count) => count + pageSize);
              }}
            >
              {query.trim() === "" && <ComboboxItem value="browser">{t("timeZoneBrowser")}</ComboboxItem>}
              {query.trim() === "" && <ComboboxItem value="utc">{t("timeZoneUtc")}</ComboboxItem>}
              {candidates.map((zone) => (
                <ComboboxItem key={zone} value={zone}>
                  {zone}
                </ComboboxItem>
              ))}
            </ComboboxList>
            <ComboboxEmpty>{t("timeZoneEmpty")}</ComboboxEmpty>
          </ComboboxContent>
        </Combobox>
      </CardContent>
    </Card>
  );
}
