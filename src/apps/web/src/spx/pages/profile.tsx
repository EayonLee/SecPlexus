"use client";

import Link from "next/link";

import { Building2 } from "lucide-react";
import { useTranslations } from "next-intl";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";

import { useIdentity } from "../iam/session";

export function ProfilePage() {
  const t = useTranslations("Account");
  const auth = useIdentity();
  const initial = auth.principal?.display_name.slice(0, 1) ?? "";
  return (
    <div className="@container/main flex flex-col gap-4 md:gap-6">
      <header>
        <h1 className="font-semibold text-2xl tracking-tight">{t("profile")}</h1>
      </header>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="flex flex-col gap-4 lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle>{t("displayName")}</CardTitle>
              <CardDescription>{t("profileIdentity")}</CardDescription>
            </CardHeader>
            <CardContent>
              <Input id="profile-name" value={auth.principal?.display_name ?? ""} readOnly />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("organizations")}</CardTitle>
            </CardHeader>
            <CardContent>
              {auth.organizations.length ? (
                <ul className="flex flex-col">
                  {auth.organizations.map((org, index) => (
                    <li key={org.id}>
                      {index > 0 ? <Separator /> : null}
                      <Link
                        href={`/dashboard?org_id=${encodeURIComponent(org.id)}`}
                        className="hover:bg-muted/50 flex items-center gap-3 rounded-md py-3 text-sm underline-offset-4 transition-colors hover:underline"
                      >
                        <Building2 className="text-muted-foreground size-4" />
                        {org.name}
                      </Link>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-muted-foreground text-sm">{t("noOrganizations")}</p>
              )}
            </CardContent>
          </Card>
        </div>

        <Card className="h-fit">
          <CardHeader>
            <CardTitle>{t("profileIdentity")}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center gap-4 text-center">
            <Avatar className="size-24">
              <AvatarFallback className="text-2xl">{initial}</AvatarFallback>
            </Avatar>
            <div className="flex flex-col gap-1">
              <p className="font-medium">{auth.principal?.display_name}</p>
              <p className="text-muted-foreground text-sm">@{auth.principal?.username}</p>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
