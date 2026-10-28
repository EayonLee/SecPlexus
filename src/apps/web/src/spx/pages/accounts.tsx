"use client";
import { useState } from "react";

import { useTranslations } from "next-intl";

import type { components } from "@/auth/api.gen";
import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

import { Can } from "../components/access";
import { DataTable, useList } from "../components/data";
import { FormDialog, type FormField } from "../components/forms";
import { api, refreshIdentity } from "../iam/session";

type User = components["schemas"]["APIUser"];
export function AccountsPage() {
  const t = useTranslations("Management");

  const state = useList<User>("/iam/users");
  const [dialog, setDialog] = useState<
    { action: "create" } | { action: "edit" | "reset" | "super"; user: User } | null
  >(null);
  let fields: FormField[] = [];
  if (dialog?.action === "create" || dialog?.action === "edit") {
    fields = [
      ...(dialog.action === "create"
        ? [{ name: "username", label: t("username"), required: true, pattern: "[A-Za-z][A-Za-z0-9_.-]{2,63}" }]
        : []),
      {
        name: "display_name",
        label: t("displayName"),
        required: true,
        maxLength: 100,
        value: dialog.action === "edit" ? dialog.user.display_name : "",
      },
      {
        name: "email",
        label: t("email"),
        type: "email",
        value: dialog.action === "edit" ? (dialog.user.email ?? "") : "",
      },
      ...(dialog.action === "create"
        ? [
            {
              name: "password",
              label: t("password"),
              type: "password" as const,
              required: true,
              minLength: 8,
              maxLength: 128,
            },
          ]
        : [
            {
              name: "status",
              label: t("status"),
              value: dialog.user.status,
              options: [
                { label: t("active"), value: "active" },
                { label: t("disabled"), value: "disabled" },
              ],
            },
          ]),
    ];
  } else if (dialog?.action === "reset")
    fields = [
      {
        name: "password",
        label: t("newPassword"),
        type: "password",
        required: true,
        minLength: 8,
        maxLength: 128,
        description: t("resetDescription"),
      },
    ];
  else if (dialog?.action === "super")
    fields = [{ name: "reason", label: t("reason"), type: "textarea", required: true, maxLength: 500 }];
  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        <Can action={permissions.IAMPlatformUsersManage}>
          <Button onClick={() => setDialog({ action: "create" })}>{t("createAccount")}</Button>
        </Can>
        <Button variant="outline" onClick={state.reload}>
          {t("refresh")}
        </Button>
      </div>
      <DataTable
        state={state}
        columns={[
          { title: t("username"), cell: (user) => user.username },
          { title: t("displayName"), cell: (user) => user.display_name },
          {
            title: t("status"),
            cell: (user) => <Badge variant="secondary">{user.status === "active" ? t("active") : t("disabled")}</Badge>,
          },
          { title: t("platformRole"), cell: (user) => (user.is_super_admin ? t("superAdmin") : t("regularUser")) },
          {
            title: t("action"),
            cell: (user) => (
              <div className="flex gap-2">
                <Can action={permissions.IAMPlatformUsersManage}>
                  <Button variant="ghost" onClick={() => setDialog({ action: "edit", user })}>
                    {t("edit")}
                  </Button>
                  <Button variant="ghost" onClick={() => setDialog({ action: "reset", user })}>
                    {t("resetPassword")}
                  </Button>
                </Can>
                <Can action={permissions.IAMPlatformSuperAdminsManage}>
                  <Button variant="ghost" onClick={() => setDialog({ action: "super", user })}>
                    {user.is_super_admin ? t("removeSuperAdmin") : t("makeSuperAdmin")}
                  </Button>
                </Can>
              </div>
            ),
          },
        ]}
      />
      {dialog && (
        <FormDialog
          title={
            {
              create: t("createPlatformAccount"),
              edit: t("editPlatformAccount"),
              reset: t("resetPassword"),
              super: t("changeSuperAdmin"),
            }[dialog.action]
          }
          fields={fields}
          onClose={() => setDialog(null)}
          onSubmit={async (values) => {
            if (dialog.action === "create") await api("/iam/users", { method: "POST", body: values });
            else if (dialog.action === "edit")
              await api(`/iam/users/${dialog.user.id}`, {
                method: "PATCH",
                body: { ...values, auth_version: dialog.user.auth_version },
              });
            else if (dialog.action === "reset")
              await api(`/iam/users/${dialog.user.id}/reset-password`, { method: "POST", body: values });
            else
              await api(`/iam/users/${dialog.user.id}/super-admin`, {
                method: "PUT",
                body: { enabled: !dialog.user.is_super_admin, reason: values.reason },
              });
            state.reload();
            await refreshIdentity();
          }}
        />
      )}
    </div>
  );
}
