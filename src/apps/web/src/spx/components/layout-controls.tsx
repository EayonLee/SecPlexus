"use client";
import { Palette } from "lucide-react";
import { useTranslations } from "next-intl";
import { useShallow } from "zustand/react/shallow";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { fontOptions } from "@/lib/fonts/registry";
import { PREFERENCE_REGISTRY, type PreferenceKey, type PreferenceValueMap } from "@/lib/preferences/preferences-config";
import { PREFERENCE_TOGGLES } from "@/lib/preferences/preferences-toggles";
import { usePreferencesStore } from "@/stores/preferences/preferences-provider";

export function LayoutControls() {
  const t = useTranslations("Appearance");

  const { values, setPreference, resetPreferences } = usePreferencesStore(
    useShallow((state) => ({
      values: state.values,
      setPreference: state.setPreference,
      resetPreferences: state.resetPreferences,
    })),
  );

  // 只接受注册表声明过的取值：DOM 里的 value 是字符串，越界值不得写进偏好。
  const commit = <K extends PreferenceKey>(key: K, value: string) => {
    const allowed: readonly string[] = PREFERENCE_REGISTRY[key].values;
    if (!allowed.includes(value)) return;
    setPreference(key, value as PreferenceValueMap[K]);
  };

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t("title")} title={t("title")}>
          <Palette />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end">
        <div className="flex flex-col gap-5">
          <div className="space-y-1.5">
            <h4 className="text-sm leading-none font-medium">{t("title")}</h4>
            <p className="text-muted-foreground text-xs">{t("description")}</p>
          </div>
          <div className="space-y-3 **:data-[slot=toggle-group]:w-full **:data-[slot=toggle-group-item]:flex-1 **:data-[slot=toggle-group-item]:text-xs">
            <div className="space-y-1">
              <Label className="text-xs font-medium">{t("fonts")}</Label>
              <Select value={values.font} onValueChange={(value) => commit("font", value)}>
                <SelectTrigger size="sm" className="w-full text-xs">
                  <SelectValue placeholder={t("selectFont")} />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {fontOptions.map((font) => (
                      <SelectItem key={font.key} className="text-xs" value={font.key}>
                        {font.label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>

            {PREFERENCE_TOGGLES.map(({ preference, label, options }) => (
              <div key={preference} className="space-y-1">
                <Label className="text-xs font-medium">{t(label)}</Label>
                <ToggleGroup
                  size="sm"
                  spacing={0}
                  variant="outline"
                  type="single"
                  value={values[preference]}
                  onValueChange={(value) => commit(preference, value)}
                >
                  {options.map(({ value, label: optionLabel }) => (
                    <ToggleGroupItem key={value} value={value} aria-label={t(optionLabel)}>
                      {t(optionLabel)}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </div>
            ))}

            <Button type="button" size="sm" variant="outline" className="w-full text-xs" onClick={resetPreferences}>
              {t("restore")}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
