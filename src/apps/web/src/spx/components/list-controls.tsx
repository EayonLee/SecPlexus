"use client";

import { type ComponentProps, type ReactNode, useState } from "react";

import Link from "next/link";

import { cn } from "cn";
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, SearchIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { ComboboxContent, ComboboxInput, ComboboxItem, ComboboxTrigger } from "@/components/ui/combobox";
import { InputGroupAddon } from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  selectTriggerStyles,
} from "@/components/ui/select";

/** 长名称保留可访问全称，在窄屏为状态、引用和行操作留出空间。 */
export function ListRecordLink({ href, name }: { href: string; name: string }) {
  return (
    <Button
      asChild
      variant="link"
      className="block h-auto max-w-[clamp(6rem,28cqi,21rem)] truncate p-0 text-start font-medium"
    >
      <Link href={href} title={name}>
        {name}
      </Link>
    </Button>
  );
}

function ListFilterLabel({ label }: { label: string }) {
  const t = useTranslations("Common");
  return (
    <span aria-hidden="true" className="text-muted-foreground shrink-0">
      {t.has("filterLabel") ? t("filterLabel", { label }) : label}
    </span>
  );
}

export function useListSearchState() {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState<string | null>(null);
  return {
    open,
    text,
    onOpenChange(next: boolean) {
      setOpen(next);
      if (!next) setText(null);
    },
    onInputValueChange(value: string, event: { reason: string }) {
      if (event.reason === "input-change") setText(value);
    },
  };
}

export function ListSearchTrigger({ label, value }: { label: string; value?: string }) {
  const t = useTranslations("Common");
  const display = value ?? t("allOptions");
  return (
    <ComboboxTrigger
      data-size="sm"
      className={cn(selectTriggerStyles, "min-w-[min(12rem,100%)] max-w-[min(20rem,100%)]")}
      aria-label={label}
      title={display}
    >
      <ListFilterLabel label={label} />
      <span className="min-w-0 flex-1 truncate text-start">{display}</span>
    </ComboboxTrigger>
  );
}

export function ListSearchContent({ label, children }: { label: string; children: ReactNode }) {
  const t = useTranslations("Common");
  return (
    <ComboboxContent
      aria-label={label}
      className="w-max min-w-[min(max(var(--anchor-width),13rem),var(--available-width))] max-w-[min(22rem,var(--available-width))]"
    >
      <ComboboxInput
        aria-label={t("searchFilter", { label })}
        placeholder={t("searchFilter", { label })}
        className="min-w-0 [&>input]:truncate"
        maxLength={255}
        showTrigger={false}
      >
        <InputGroupAddon>
          <SearchIcon />
        </InputGroupAddon>
      </ComboboxInput>
      {children}
    </ComboboxContent>
  );
}

export function ListSearchOption({ label, ...props }: ComponentProps<typeof ComboboxItem> & { label: string }) {
  return (
    <ComboboxItem className="data-selected:bg-accent" {...props}>
      <span title={label} className="min-w-0 truncate">
        {label}
      </span>
    </ComboboxItem>
  );
}

export function ListSelect<T extends string>({
  label,
  value,
  options,
  onChange,
  side,
  className,
  hideLabel = false,
}: {
  label: string;
  value: T;
  options: readonly { value: T; label: string }[];
  onChange: (value: T) => void;
  side?: "top" | "bottom";
  className?: string;
  hideLabel?: boolean;
}) {
  const t = useTranslations("Common");
  const selectedLabel =
    !hideLabel && value === "all" && t.has("allOptions")
      ? t("allOptions")
      : options.find((option) => option.value === value)?.label;
  return (
    <Select
      value={value}
      onValueChange={(next) => {
        const selected = options.find((item) => item.value === next);
        if (selected) onChange(selected.value);
      }}
    >
      <SelectTrigger size="sm" className={cn("min-w-0 max-w-full", className)} aria-label={label}>
        {!hideLabel && <ListFilterLabel label={label} />}
        <SelectValue>{selectedLabel}</SelectValue>
      </SelectTrigger>
      <SelectContent side={side} align="start">
        <SelectGroup>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

export const pageSizes = [10, 20, 30, 40, 50];
const sizeOptions = pageSizes.map((size) => ({ value: String(size), label: String(size) }));

/** 总数分页和游标分页共用控件。只有已知总数时才可跳到末页。 */
export function ListPagination({
  index,
  size,
  total,
  hasNext,
  onPage,
  onSize,
  loading = false,
}: {
  index: number;
  size: number;
  total?: number;
  hasNext: boolean;
  onPage: (index: number) => void;
  onSize: (size: number) => void;
  loading?: boolean;
}) {
  const t = useTranslations("Common");
  const canMoveBack = index > 0 && !loading;
  const canMoveForward = hasNext && !loading;
  const pages = total === undefined ? undefined : Math.max(1, Math.ceil(total / size));
  return (
    <nav aria-label={t("paginationLabel")} className="flex flex-wrap items-center justify-between gap-3 p-4">
      <p className="text-muted-foreground text-sm tabular-nums">
        {total === undefined ? t("currentPage", { page: index + 1 }) : t("recordCount", { total })}
      </p>
      <div className="ml-auto flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2">
          <span className="whitespace-nowrap text-sm">{t("rowsPerPage")}</span>
          <ListSelect
            label={t("rowsPerPage")}
            hideLabel
            value={String(size)}
            options={sizeOptions}
            side="top"
            className="w-20"
            onChange={(v) => onSize(Number(v))}
          />
        </div>
        {pages !== undefined && (
          <span className="whitespace-nowrap text-sm tabular-nums">
            {t("pageInfo", { page: index + 1, total: pages })}
          </span>
        )}
        <div className="flex items-center gap-1">
          {pages !== undefined && (
            <Button
              size="icon-sm"
              variant="outline"
              aria-label={t("firstPage")}
              disabled={!canMoveBack}
              onClick={() => onPage(0)}
            >
              <ChevronsLeft />
            </Button>
          )}
          <Button
            size="icon-sm"
            variant="outline"
            aria-label={t("previousPage")}
            disabled={!canMoveBack}
            onClick={() => onPage(index - 1)}
          >
            <ChevronLeft />
          </Button>
          <Button
            size="icon-sm"
            variant="outline"
            aria-label={t("nextPage")}
            disabled={!canMoveForward}
            onClick={() => onPage(index + 1)}
          >
            <ChevronRight />
          </Button>
          {pages !== undefined && (
            <Button
              size="icon-sm"
              variant="outline"
              aria-label={t("lastPage")}
              disabled={!canMoveForward}
              onClick={() => onPage(pages - 1)}
            >
              <ChevronsRight />
            </Button>
          )}
        </div>
      </div>
    </nav>
  );
}
