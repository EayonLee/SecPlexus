"use client";
import { useState } from "react";

import { useTranslations } from "next-intl";

import type { components } from "@/auth/api.gen";
import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";

import { Can } from "../components/access";
import { DataTable, useList } from "../components/data";
import { FormDialog, type FormField } from "../components/forms";
import { UserPicker } from "../components/user-picker";
import { api, refreshIdentity } from "../iam/session";

type Organization = components["schemas"]["APIOrganization"];
export function OrganizationsPage() {
  const t = useTranslations("Management");

  const state = useList<Organization>("/iam/organizations");
  const [dialog, setDialog] = useState<"create" | Organization>();
  const creating = dialog === "create";
  const fields: FormField[] = [
    {
      name: "name",
      label: t("orgName"),
      required: true,
      maxLength: 100,
      value: typeof dialog === "object" ? dialog.name : "",
    },
    ...(creating
      ? [
          {
            name: "slug",
            label: t("orgCode"),
            required: true,
            pattern: "[a-z0-9][a-z0-9-]{1,61}[a-z0-9]",
            placeholder: t("exampleOrg"),
          },
        ]
      : [
          {
            name: "status",
            label: t("status"),
            value: typeof dialog === "object" ? dialog.status : "active",
            options: [
              { label: t("active"), value: "active" },
              { label: t("disabled"), value: "suspended" },
            ],
          },
        ]),
  ];
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
      {dialog && (
        <FormDialog
          title={creating ? t("createOrg") : t("editOrg")}
          fields={fields}
          onClose={() => setDialog(undefined)}
          onSubmit={async (values) => {
            if (dialog === "create") {
              if (!values.admin_user_id) throw new Error(t("chooseInitialAdmin"));
              await api("/iam/organizations", { method: "POST", body: values });
            } else
              await api(`/iam/organizations/${dialog.id}`, {
                method: "PATCH",
                body: { ...values, authz_version: dialog.authz_version },
              });
            state.reload();
            await refreshIdentity();
          }}
        >
          {creating && (
            <Field>
              <FieldLabel>{t("initialAdmin")}</FieldLabel>
              <UserPicker name="admin_user_id" label={t("initialAdmin")} required />
              <FieldDescription>{t("initialAdminDescription")}</FieldDescription>
            </Field>
          )}
        </FormDialog>
      )}
    </div>
  );
}
