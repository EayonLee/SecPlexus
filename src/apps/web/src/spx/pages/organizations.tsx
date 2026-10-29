"use client";
import { useState } from "react";

import { useMutation } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { Controller } from "react-hook-form";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { createOrganization, listOrganizations, updateOrganization } from "@/spx/api/iam/management";
import type { ApiOrganization } from "@/spx/api/iam/schemas";
import { zCreateOrganizationBody, zUpdateOrganizationBody } from "@/spx/api/iam/schemas";

import { Can } from "../components/access";
import { DataTable, usePagedQuery } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
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
            cell: (org) => <Badge variant="secondary">{org.status === "active" ? t("active") : t("disabled")}</Badge>,
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
              <FormInput label={t("orgName")} registration={form.register("name")} error={form.formState.errors.name} />
              <FormInput
                label={t("orgCode")}
                registration={form.register("slug")}
                error={form.formState.errors.slug}
                placeholder={t("exampleOrg")}
              />
              <Field>
                <FieldLabel>{t("initialAdmin")}</FieldLabel>
                <Controller
                  control={form.control}
                  name="admin_user_id"
                  render={({ field }) => (
                    <UserPicker label={t("initialAdmin")} value={field.value} onValueChange={field.onChange} />
                  )}
                />
                <FieldDescription>{t("initialAdminDescription")}</FieldDescription>
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
              <FormInput label={t("orgName")} registration={form.register("name")} error={form.formState.errors.name} />
              <Field>
                <FieldLabel>{t("status")}</FieldLabel>
                <NativeSelect {...form.register("status")}>
                  <NativeSelectOption value="active">{t("active")}</NativeSelectOption>
                  <NativeSelectOption value="suspended">{t("disabled")}</NativeSelectOption>
                </NativeSelect>
              </Field>
            </>
          )}
        </FormDialog>
      )}
    </div>
  );
}
