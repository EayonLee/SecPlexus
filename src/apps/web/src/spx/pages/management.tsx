"use client";
import { useEffect } from "react";

import { usePathname, useRouter } from "next/navigation";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useFormatter, useTranslations } from "next-intl";
import { useForm } from "react-hook-form";
import type { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  getOrganization,
  listAuditEvents,
  listOrganizationAuditEvents,
  securityOverview,
  updateCurrentOrganization,
} from "@/spx/api/iam/management";
import type { ApiAuditEvent, ApiOrganization } from "@/spx/api/iam/schemas";
import { zUpdateCurrentOrganizationBody } from "@/spx/api/iam/schemas";

import { AccessDenied, Can, RequireAccess } from "../components/access";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { managementChanged } from "../iam/management";
import { managementPages, visibleManagementPages } from "../iam/navigation";
import { identityScope } from "../iam/query";
import { useIdentity } from "../iam/session";
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
  const t = useTranslations("Management");

  const auth = useIdentity();
  const org = auth.org_id ?? "";
  const organization = useQuery({
    queryKey: ["iam", "organization", org, ...identityScope(auth)],
    queryFn: ({ signal }) => getOrganization(org, signal),
    enabled: !!org,
  });
  const data = organization.data;
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
        <OrganizationNameForm key={data.authz_version} organization={data} />
      </Can>
    </div>
  );
}
function AuditPage({ platform }: { platform: boolean }) {
  const t = useTranslations("Management");
  const format = useFormatter();

  const org = useIdentity((state) => state.org_id) ?? "";
  const state = usePagedQuery<ApiAuditEvent>(["audit", platform, org], (query, signal) =>
    platform ? listAuditEvents(query, signal) : listOrganizationAuditEvents(org, query, signal),
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
  const t = useTranslations("Management");

  const auth = useIdentity();
  const query = useQuery({
    queryKey: ["iam", "security", ...identityScope(auth)],
    queryFn: ({ signal }) => securityOverview(signal),
  });
  const data = query.data;
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

function OrganizationNameForm({ organization }: { organization: ApiOrganization }) {
  const t = useTranslations("Management");
  const errors = useTranslations("Errors");
  const perform = useAction();
  const edit = useMutation({
    mutationFn: updateCurrentOrganization,
    onSuccess: managementChanged,
  });
  const form = useForm<z.infer<typeof zUpdateCurrentOrganizationBody>>({
    resolver: zodResolver(zUpdateCurrentOrganizationBody),
    defaultValues: { name: organization.name, authz_version: organization.authz_version },
  });
  return (
    <form
      className="max-w-lg"
      noValidate
      onSubmit={form.handleSubmit(async (body) => {
        await perform(async () => {
          await edit.mutateAsync({
            orgId: organization.id,
            body,
          });
        }, t("orgNameSaved"));
      })}
    >
      <fieldset disabled={form.formState.isSubmitting}>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="org-name">{t("orgName")}</FieldLabel>
            <Input id="org-name" {...form.register("name")} aria-invalid={!!form.formState.errors.name} />
            {form.formState.errors.name && <FieldError>{errors("invalidRequest")}</FieldError>}
          </Field>
          <Button type="submit">{t("saveOrg")}</Button>
        </FieldGroup>
      </fieldset>
    </form>
  );
}
