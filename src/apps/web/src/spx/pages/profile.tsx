"use client";
import { useFormatter, useTranslations } from "next-intl";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

import { useIdentity } from "../iam/session";

export function ProfilePage() {
  const t = useTranslations("Account");
  const format = useFormatter();

  const principal = useIdentity((state) => state.principal);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("profile")}</CardTitle>
        <CardDescription>{t("profileDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="grid gap-4 text-sm sm:grid-cols-2">
          {[
            [t("username"), principal?.username],
            [t("displayName"), principal?.display_name],
            [t("accountId"), principal?.user_id],
            [
              t("signedInAt"),
              principal
                ? format.dateTime(new Date(principal.authenticated_at), { dateStyle: "medium", timeStyle: "medium" })
                : "",
            ],
          ].map(([label, value]) => (
            <div key={label}>
              <dt className="text-muted-foreground">{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  );
}
