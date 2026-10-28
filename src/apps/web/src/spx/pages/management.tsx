"use client";
import { useEffect, useState } from "react";

import { usePathname, useRouter } from "next/navigation";

import { useFormatter, useTranslations } from "next-intl";
import { toast } from "sonner";

import type { components } from "@/auth/api.gen";
import { permissions } from "@/auth/permissions.gen";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

import { AccessDenied, Can, RequireAccess } from "../components/access";
import { DataTable, useAction, useList } from "../components/data";
import { managementPages, visibleManagementPages } from "../iam/navigation";
import { api, refreshIdentity, useIdentity } from "../iam/session";
import { AccountsPage } from "./accounts";
import { MembersPage } from "./members";
import { OrganizationsPage } from "./organizations";
import { RolesPage } from "./roles";

export function ManagementPage() {
  const nav = useTranslations("Navigation");
  const t = useTranslations("Management");

  const auth = useIdentity();
  const pathname = usePathname();
  const router = useRouter();
  const page = managementPages.find((entry) => entry.path(auth.org_id) === pathname);
  useEffect(() => {
    if (pathname === "/management") {
      const first = visibleManagementPages(auth)[0];
      if (first) router.replace(first.path(auth.org_id));
    }
  }, [pathname, auth, router]);
  if (!page) return <AccessDenied />;
  return (
    <RequireAccess action={page.action}>
      <div className="flex flex-col gap-6">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{nav(page.title)}</h1>
          <p className="text-muted-foreground text-sm">{nav(page.group)}</p>
        </div>
        <Card>
          <CardHeader>
            <CardTitle>{nav(page.title)}</CardTitle>
            <CardDescription>
              {page.scope === "organization" ? t("orgDescription") : t("platformDescription")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {page.id === "users" && <AccountsPage />}
            {page.id === "organizations" && <OrganizationsPage />}
            {page.id === "members" && <MembersPage key={auth.org_id} />}
            {page.id === "roles" && <RolesPage key={auth.org_id} />}
            {page.id === "overview" && <OverviewPage key={auth.org_id} />}
            {(page.id === "audit" || page.id === "platform-audit") && (
              <AuditPage key={page.id} platform={page.id === "platform-audit"} />
            )}
            {page.id === "security" && <SecurityOverview />}
          </CardContent>
        </Card>
      </div>
    </RequireAccess>
  );
}
function OverviewPage() {
  const perform = useAction();

  const t = useTranslations("Management");

  const org = useIdentity((state) => state.org_id);
  const [data, setData] = useState<components["schemas"]["APIOrganization"]>();
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: 保存后通过 revision 刷新服务端组织版本。
  useEffect(() => {
    const controller = new AbortController();
    void perform(async () => {
      const result = await api<components["schemas"]["APIOrganization"]>(`/iam/orgs/${org}`, {
        signal: controller.signal,
      });
      if (!controller.signal.aborted) setData(result);
    });
    return () => controller.abort();
  }, [org, revision]);
  if (!data) return null;
  return (
    <div className="flex flex-col gap-6">
      <dl className="grid gap-4 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-muted-foreground">{t("orgId")}</dt>
          <dd>{data.id}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("orgCode")}</dt>
          <dd>{data.slug}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("status")}</dt>
          <dd>{data.status === "active" ? t("active") : t("disabled")}</dd>
        </div>
      </dl>
      <Can action={permissions.IAMOrganizationsManage}>
        <form
          className="max-w-lg"
          onSubmit={(event) => {
            event.preventDefault();
            const values = new FormData(event.currentTarget);
            void perform(async () => {
              await api(`/iam/orgs/${org}`, {
                method: "PATCH",
                body: { name: values.get("name"), authz_version: data.authz_version },
              });
              await refreshIdentity(org);
              setRevision((value) => value + 1);
              toast.success(t("orgNameSaved"));
            });
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="org-name">{t("orgName")}</FieldLabel>
              <Input
                key={data.authz_version}
                id="org-name"
                name="name"
                defaultValue={data.name}
                required
                maxLength={100}
              />
            </Field>
            <Button type="submit">{t("saveOrg")}</Button>
          </FieldGroup>
        </form>
      </Can>
    </div>
  );
}
function AuditPage({ platform }: { platform: boolean }) {
  const t = useTranslations("Management");
  const format = useFormatter();

  const org = useIdentity((state) => state.org_id);
  const state = useList<components["schemas"]["APIAuditEvent"]>(
    platform ? "/iam/audit-events" : `/iam/orgs/${org}/audit-events`,
  );
  return (
    <DataTable
      state={state}
      columns={[
        {
          title: t("time"),
          cell: (row) => format.dateTime(new Date(row.occurred_at), { dateStyle: "medium", timeStyle: "medium" }),
        },
        { title: t("actor"), cell: (row) => row.actor_name || t("system") },
        { title: t("action"), cell: (row) => row.action },
        {
          title: t("outcome"),
          cell: (row) => (
            <Badge variant="secondary">
              {({ success: t("success"), failure: t("failure"), denied: t("denied") } as Record<string, string>)[
                row.outcome
              ] ?? row.outcome}
            </Badge>
          ),
        },
        { title: t("requestId"), cell: (row) => row.request_id },
      ]}
    />
  );
}
function SecurityOverview() {
  const perform = useAction();

  const t = useTranslations("Management");

  const [data, setData] = useState<components["schemas"]["APISecurityOverview"]>();
  useEffect(() => {
    const controller = new AbortController();
    void perform(async () => {
      const result = await api<components["schemas"]["APISecurityOverview"]>("/iam/security/overview", {
        signal: controller.signal,
      });
      if (!controller.signal.aborted) setData(result);
    });
    return () => controller.abort();
  }, [perform]);
  return (
    <div className="flex flex-col gap-6">
      <Alert>
        <AlertDescription>{t("securityDescription")}</AlertDescription>
      </Alert>
      {data && (
        <dl className="grid gap-4 text-sm sm:grid-cols-2">
          {[
            [t("idleTimeout"), data.session_idle_timeout],
            [t("absoluteTimeout"), data.session_absolute_timeout],
            [t("reauthWindow"), data.reauth_window],
            [t("maxFailures"), data.login_max_failures],
            [t("lockDuration"), data.login_lock_duration],
            [t("selfRegistration"), data.self_registration ? t("enabled") : t("off")],
          ].map(([label, value]) => (
            <div key={label}>
              <dt className="text-muted-foreground">{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  );
}
