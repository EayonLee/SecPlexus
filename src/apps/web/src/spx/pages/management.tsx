"use client";
import { useEffect } from "react";

import { usePathname, useRouter } from "next/navigation";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useFormatter, useTranslations } from "next-intl";
import { useForm } from "react-hook-form";
import type { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  getOrganization,
  listAuditEvents,
  listOrganizationAuditEvents,
  updateCurrentOrganization,
} from "@/spx/api/iam/management";
import type { ApiAuditEvent, ApiOrganization } from "@/spx/api/iam/schemas";
import { zUpdateCurrentOrganizationBody } from "@/spx/api/iam/schemas";

import { AccessDenied, RequireAccess, useCan } from "../components/access";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { StatusBadge } from "../components/status-badge";
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
  if (pathname === "/management") {
    return visibleManagementPages(auth).length ? <Skeleton className="h-48 w-full" /> : <AccessDenied />;
  }
  if (!page) return <AccessDenied />;
  const tabs = visibleManagementPages(auth).filter((entry) => entry.group === page.group);
  return (
    <RequireAccess action={page.action}>
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">{nav(page.group)}</h1>
        <Tabs
          value={page.id}
          onValueChange={(id) => {
            const target = tabs.find((entry) => entry.id === id);
            if (target) router.push(target.path(auth.org_id));
          }}
        >
          <TabsList variant="line" className="mb-2">
            {tabs.map((entry) => (
              <TabsTrigger key={entry.id} value={entry.id}>
                {nav(entry.title)}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value={page.id} className="flex flex-col gap-6">
            {page.id === "users" && <AccountsPage />}
            {page.id === "organizations" && <OrganizationsPage />}
            {page.id === "members" && <MembersPage key={auth.org_id} />}
            {page.id === "roles" && <RolesPage key={auth.org_id} />}
            {page.id === "overview" && <OverviewPage key={auth.org_id} />}
            {(page.id === "audit" || page.id === "platform-audit") && (
              <AuditPage key={`${page.id}:${auth.org_id}`} platform={page.id === "platform-audit"} />
            )}
          </TabsContent>
        </Tabs>
      </div>
    </RequireAccess>
  );
}
function OverviewPage() {
  const t = useTranslations("Management");
  const canManage = useCan(permissions.IAMOrganizationsManage);

  const auth = useIdentity();
  const org = auth.org_id ?? "";
  const organization = useQuery({
    queryKey: ["iam", "organization", org, ...identityScope(auth)],
    queryFn: ({ signal }) => getOrganization(org, signal),
    enabled: !!org,
  });
  const data = organization.data;
  if (organization.isError) return <p role="alert">{t("loadOrganizationFailed")}</p>;
  if (!data) return <Skeleton className="h-48 w-full" />;
  return (
    <div className="flex max-w-3xl flex-col gap-8">
      {canManage ? (
        <OrganizationNameForm key={data.authz_version} organization={data} />
      ) : (
        <Field className="max-w-xl">
          <FieldLabel htmlFor="organization-name">{t("orgName")}</FieldLabel>
          <Input id="organization-name" value={data.name} readOnly />
        </Field>
      )}
      <section className="flex flex-col gap-4">
        <h2 className="border-b pb-3 text-lg font-medium">{t("organizationDetails")}</h2>
        <dl className="grid grid-cols-2 gap-6 text-sm">
          <div className="flex flex-col gap-2">
            <dt className="font-medium">{t("status")}</dt>
            <dd>
              <StatusBadge status={data.status}>{data.status === "active" ? t("active") : t("disabled")}</StatusBadge>
            </dd>
          </div>
          <div className="flex flex-col gap-2">
            <dt className="font-medium">{t("orgCode")}</dt>
            <dd className="text-muted-foreground">{data.slug}</dd>
          </div>
          <div className="flex flex-col gap-2">
            <dt className="font-medium">{t("orgId")}</dt>
            <dd className="text-muted-foreground font-mono text-xs">{data.id}</dd>
          </div>
        </dl>
      </section>
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
            <StatusBadge status={row.outcome}>
              {({ success: t("success"), failure: t("failure"), denied: t("denied") } as Record<string, string>)[
                row.outcome
              ] ?? row.outcome}
            </StatusBadge>
          ),
        },
        { title: t("requestId"), cell: (row) => row.request_id },
      ]}
    />
  );
}
function OrganizationNameForm({ organization }: { organization: ApiOrganization }) {
  const t = useTranslations("Management");
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
      className="max-w-xl"
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
      <fieldset disabled={form.formState.isSubmitting} className="max-w-xl">
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="org-name">{t("orgName")}</FieldLabel>
            <Input id="org-name" {...form.register("name")} aria-invalid={!!form.formState.errors.name} />
            {form.formState.errors.name && <FieldError>{t("invalidOrgName")}</FieldError>}
          </Field>
          <Field orientation="horizontal">
            <Button type="submit" disabled={!form.formState.isDirty || form.formState.isSubmitting}>
              {t("saveOrg")}
            </Button>
          </Field>
        </FieldGroup>
      </fieldset>
    </form>
  );
}
