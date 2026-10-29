"use client";
import { useEffect, useState } from "react";

import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";

import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { listUsers } from "@/spx/api/iam/management";

import { identityScope } from "../iam/query";
import { useIdentity } from "../iam/session";

export function UserPicker({
  name = "user_id",
  label,
  required = false,
  value,
  onValueChange,
}: {
  name?: string;
  label?: string;
  required?: boolean;
  value?: string;
  onValueChange?: (value: string) => void;
}) {
  const t = useTranslations("Common");

  const [query, setQuery] = useState("");
  const auth = useIdentity();
  const [search, setSearch] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setSearch(query), 200);
    return () => clearTimeout(timer);
  }, [query]);
  const results = useQuery({
    queryKey: ["iam", "users", search, ...identityScope(auth)],
    queryFn: ({ signal }) => listUsers({ q: search, limit: 50 }, signal),
  });
  const users = results.data?.items ?? [];

  return (
    <div className="flex flex-col gap-2">
      <Input
        aria-label={t("searchUsers")}
        placeholder={t("searchUsersPlaceholder")}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
      />
      <Select name={name} required={required} value={value} onValueChange={onValueChange}>
        <SelectTrigger aria-label={label ?? t("chooseUser")}>
          <SelectValue placeholder={t("chooseActiveUser")} />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {users.map((user) => (
              <SelectItem key={user.id} value={user.id} disabled={user.status !== "active"}>
                {user.display_name} ({user.username})
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  );
}
