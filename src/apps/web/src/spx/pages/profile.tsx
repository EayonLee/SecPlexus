"use client";

import Link from "next/link";

import { Building2 } from "lucide-react";
import { useTranslations } from "next-intl";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";

import { useIdentity } from "../iam/session";

export function ProfilePage() {
  const t = useTranslations("Account");
  const auth = useIdentity();
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">{t("profile")}</h1>
      <Separator />
      <div className="grid grid-cols-[minmax(0,1fr)_180px] gap-16">
        <div className="flex max-w-2xl flex-col gap-8">
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="profile-name">{t("displayName")}</FieldLabel>
              <Input id="profile-name" value={auth.principal?.display_name ?? ""} readOnly />
            </Field>
          </FieldGroup>
          <section className="flex flex-col gap-3">
            <h2 className="text-base font-medium">{t("organizations")}</h2>
            <Separator />
            {auth.organizations.length ? (
              <ul className="divide-y">
                {auth.organizations.map((org) => (
                  <li key={org.id}>
                    <Link
                      href={`/dashboard?org_id=${encodeURIComponent(org.id)}`}
                      className="flex items-center gap-3 py-3 text-sm underline-offset-4 hover:underline"
                    >
                      <Building2 className="text-muted-foreground size-4" />
                      {org.name}
                    </Link>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-muted-foreground text-sm whitespace-nowrap">{t("noOrganizations")}</p>
            )}
          </section>
        </div>
        <aside className="flex flex-col items-start gap-4">
          <h2 className="text-sm font-medium">{t("profileIdentity")}</h2>
          <Avatar className="size-32">
            <AvatarFallback className="text-3xl">{auth.principal?.display_name.slice(0, 1)}</AvatarFallback>
          </Avatar>
          <div className="flex flex-col gap-1">
            <p className="font-medium">{auth.principal?.display_name}</p>
            <p className="text-muted-foreground text-sm">@{auth.principal?.username}</p>
          </div>
        </aside>
      </div>
    </div>
  );
}
