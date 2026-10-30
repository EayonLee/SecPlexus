"use client";
import { type Ref, useEffect, useRef, useState } from "react";

import { useInfiniteQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import { Spinner } from "@/components/ui/spinner";
import { listUsers } from "@/spx/api/iam/management";
import type { ApiUser } from "@/spx/api/iam/schemas";

import { identityScope } from "../iam/query";
import { errorMessageKey, useIdentity } from "../iam/session";

export function UserPicker({
  name = "user_id",
  label,
  required = false,
  value,
  onValueChange,
  id,
  invalid,
  onBlur,
  ref,
}: {
  id?: string;
  invalid?: boolean;
  onBlur?: () => void;
  ref?: Ref<HTMLInputElement>;
  name?: string;
  label?: string;
  required?: boolean;
  value?: string;
  onValueChange?: (value: string) => void;
}) {
  const t = useTranslations("Common");
  const errors = useTranslations("Errors");
  const auth = useIdentity();
  const container = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<ApiUser | null>(null);
  useEffect(() => {
    const timer = setTimeout(() => setSearch(query.trim()), 200);
    return () => clearTimeout(timer);
  }, [query]);
  const results = useInfiniteQuery({
    queryKey: ["iam", "user-picker", search, ...identityScope(auth)],
    enabled: open && auth.status === "authenticated",
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => listUsers({ q: search, limit: 10, offset: pageParam }, signal),
    getNextPageParam: (last) =>
      last.items.length && last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined,
  });
  const users = results.data?.pages.flatMap((page) => page.items) ?? [];
  const options = selected && !users.some((user) => user.id === selected.id) ? [selected, ...users] : users;
  return (
    <div ref={container} className="w-full">
      <Combobox
        items={options}
        filter={null}
        name={name}
        required={required}
        value={selected?.id === value ? selected : null}
        itemToStringLabel={(user) => `${user.display_name} (${user.username})`}
        itemToStringValue={(user) => user.id}
        isItemEqualToValue={(a, b) => a.id === b.id}
        open={open}
        onOpenChange={setOpen}
        onInputValueChange={(text, event) => {
          if (event.reason === "input-change" || event.reason === "input-clear") setQuery(text);
        }}
        onValueChange={(user) => {
          setSelected(user);
          if (user) setQuery("");
          onValueChange?.(user?.id ?? "");
        }}
      >
        <ComboboxInput
          className="w-full"
          id={id}
          ref={ref}
          aria-invalid={invalid}
          onBlur={onBlur}
          aria-label={label ?? t("chooseUser")}
          placeholder={t("searchUsersPlaceholder")}
        />
        <ComboboxContent container={container}>
          <ComboboxList
            aria-busy={results.isFetching}
            onScroll={(event) => {
              const list = event.currentTarget;
              if (
                list.scrollHeight - list.scrollTop - list.clientHeight < 40 &&
                results.hasNextPage &&
                !results.isFetching
              )
                void results.fetchNextPage();
            }}
          >
            {(user: ApiUser) => (
              <ComboboxItem key={user.id} value={user} disabled={user.status !== "active"}>
                {user.display_name} ({user.username})
              </ComboboxItem>
            )}
          </ComboboxList>
          {!results.isFetching && !results.isError && <ComboboxEmpty>{t("empty")}</ComboboxEmpty>}
          {results.isFetching && (
            <div role="status" className="flex items-center justify-center gap-2 p-2 text-sm">
              <Spinner />
              {t("loading")}
            </div>
          )}
          {results.isError && (
            <div className="flex flex-col gap-2 p-2">
              <p role="alert" className="text-destructive text-sm">
                {errors(errorMessageKey(results.error))}
              </p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void (results.isFetchNextPageError ? results.fetchNextPage() : results.refetch())}
              >
                {t("retry")}
              </Button>
            </div>
          )}
        </ComboboxContent>
      </Combobox>
    </div>
  );
}
