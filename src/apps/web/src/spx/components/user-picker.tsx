"use client";
import { useEffect, useState } from "react";

import { useTranslations } from "next-intl";

import type { components } from "@/auth/api.gen";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

import { api } from "../iam/session";
import { type PageData, useAction } from "./data";

export function UserPicker({
  name = "user_id",
  label,
  required = false,
}: {
  name?: string;
  label?: string;
  required?: boolean;
}) {
  const perform = useAction();

  const t = useTranslations("Common");

  const [query, setQuery] = useState("");
  const [users, setUsers] = useState<components["schemas"]["APIUser"][]>([]);
  useEffect(() => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      void perform(async () => {
        const data = await api<PageData<components["schemas"]["APIUser"]>>(
          `/iam/users?q=${encodeURIComponent(query)}&limit=50`,
          { signal: controller.signal },
        );
        if (!controller.signal.aborted) setUsers(data.items);
      });
    }, 200);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [query, perform]);
  return (
    <div className="flex flex-col gap-2">
      <Input
        aria-label={t("searchUsers")}
        placeholder={t("searchUsersPlaceholder")}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
      />
      <Select name={name} required={required}>
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
