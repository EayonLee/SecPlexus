"use client";
import { useState } from "react";

import { useMutation } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { Controller } from "react-hook-form";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { createOrganization, listOrganizations, updateOrganization } from "@/spx/api/iam/management";
import type { ApiOrganization } from "@/spx/api/iam/schemas";
import { zCreateOrganizationBody, zUpdateOrganizationBody } from "@/spx/api/iam/schemas";

import { Can } from "../components/access";
import { DataTable, usePagedQuery } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { StatusBadge } from "../components/status-badge";
import { UserPicker } from "../components/user-picker";
import { managementChanged } from "../iam/management";

export function OrganizationsPage() {
  const t = useTranslations("Management");

  const state = usePagedQuery<ApiOrganization>(["iam", "organizations"], listOrganizations);
  const create = useMutation({
    mutationFn: createOrganization,
    onSuccess: managementChanged,
  });
  const edit = useMutation({
    mutationFn: updateOrganization,
    onSuccess: managementChanged,
  });
  const [dialog, setDialog] = useState<"create" | ApiOrganization>();

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        <Can action={permissions.IAMPlatformOrganizationsManage}>
          <Button onClick={() => setDialog("create")}>{t("createOrg")}</Button>
        </Can>
        <Button variant="outline" onClick={state.reload}>
          {t("refresh")}
        </Button>
      </div>
      <DataTable
        state={state}
        columns={[
          { title: t("orgName"), cell: (org) => org.name },
          { title: t("key"), cell: (org) => org.slug },
          {
            title: t("status"),
            cell: (org) => (
              <StatusBadge status={org.status}>{org.status === "active" ? t("active") : t("disabled")}</StatusBadge>
            ),
          },
          { title: t("orgId"), cell: (org) => org.id },
          {
            title: t("action"),
            cell: (org) => (
              <Can action={permissions.IAMPlatformOrganizationsManage}>
                <Button variant="ghost" onClick={() => setDialog(org)}>
                  {t("edit")}
                </Button>
              </Can>
            ),
          },
        ]}
      />
      {dialog === "create" && (
        <FormDialog
          title={t("createOrg")}
          schema={zCreateOrganizationBody}
          defaultValues={{ name: "", slug: "", admin_user_id: "" }}
          onClose={() => setDialog(undefined)}
          onSubmit={async (body) => {
            await create.mutateAsync(body);
          }}
        >
          {(form) => (
            <>
              <FormInput
                label={t("orgName")}
                errorText={t("invalidOrgName")}
                registration={form.register("name")}
                error={form.formState.errors.name}
              />
              <FormInput
                label={t("orgCode")}
                errorText={t("invalidOrgCode")}
                registration={form.register("slug")}
                error={form.formState.errors.slug}
                placeholder={t("exampleOrg")}
              />
              <Field data-invalid={!!form.formState.errors.admin_user_id}>
                <FieldLabel htmlFor="organization-administrator">{t("initialAdmin")}</FieldLabel>
                <Controller
                  control={form.control}
                  name="admin_user_id"
                  render={({ field }) => (
                    <UserPicker
                      id="organization-administrator"
                      label={t("initialAdmin")}
                      name={field.name}
                      ref={field.ref}
                      onBlur={field.onBlur}
                      required
                      invalid={!!form.formState.errors.admin_user_id}
                      value={field.value}
                      onValueChange={field.onChange}
                    />
                  )}
                />
                {form.formState.errors.admin_user_id && <FieldError>{t("chooseInitialAdmin")}</FieldError>}
              </Field>
            </>
          )}
        </FormDialog>
      )}
      {typeof dialog === "object" && (
        <FormDialog
          title={t("editOrg")}
          schema={zUpdateOrganizationBody}
          defaultValues={{ name: dialog.name, status: dialog.status, authz_version: dialog.authz_version }}
          onClose={() => setDialog(undefined)}
          onSubmit={async (body) => {
            await edit.mutateAsync({ id: dialog.id, body });
          }}
        >
          {(form) => (
            <>
              <FormInput
                label={t("orgName")}
                errorText={t("invalidOrgName")}
                registration={form.register("name")}
                error={form.formState.errors.name}
              />
              <Field>
                <FieldLabel>{t("status")}</FieldLabel>
                <Controller
                  control={form.control}
                  name="status"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange} name={field.name}>
                      <SelectTrigger
                        id="organizations-status"
                        aria-label={t("status")}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="active">{t("active")}</SelectItem>
                          <SelectItem value="suspended">{t("disabled")}</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  )}
                />
              </Field>
            </>
          )}
        </FormDialog>
      )}
    </div>
  );
}
