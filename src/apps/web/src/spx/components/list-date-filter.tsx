"use client";

import { useState } from "react";

import { format, parseISO } from "date-fns";
import { CalendarIcon } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { enUS } from "react-day-picker/locale/en-US";
import { zhCN } from "react-day-picker/locale/zh-CN";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { resolveLocale } from "@/i18n/config";

const calendarLocales = { en: enUS, "zh-CN": zhCN };

export function ListDateFilter({
  label,
  value,
  min,
  max,
  onChange,
}: {
  label: string;
  value?: string;
  min?: string;
  max?: string;
  onChange: (day?: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const locale = calendarLocales[resolveLocale(useLocale())];
  const t = useTranslations("Common");
  const selected = value ? parseISO(value) : undefined;
  const initialMonth = value ?? min ?? max;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button size="sm" variant="outline" className="font-normal">
          <CalendarIcon data-icon="inline-start" />
          {label}
          {value && <span className="tabular-nums">{value}</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto p-0">
        <Calendar
          mode="single"
          autoFocus
          locale={locale}
          defaultMonth={initialMonth ? parseISO(initialMonth) : undefined}
          selected={selected}
          disabled={[...(min ? [{ before: parseISO(min) }] : []), ...(max ? [{ after: parseISO(max) }] : [])]}
          onSelect={(day) => {
            onChange(day ? format(day, "yyyy-MM-dd") : undefined);
            setOpen(false);
          }}
        />
        {selected && (
          <div className="border-t p-1">
            <Button
              size="sm"
              variant="ghost"
              className="w-full"
              onClick={() => {
                onChange(undefined);
                setOpen(false);
              }}
            >
              {t("clearDate")}
            </Button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
