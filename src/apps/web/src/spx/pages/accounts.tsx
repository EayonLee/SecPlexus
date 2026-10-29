"use client";
import { useState } from "react";

import { useTranslations } from "next-intl";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { listUsersQueryKey } from "@/spx/api/iam/@tanstack/react-query.gen";
import { createUser, listUsers, resetUserPassword, setSuperAdmin, updateUser } from "@/spx/api/iam/sdk.gen";
import type { ApiUser } from "@/spx/api/iam/types.gen";
import { zCreateUserBody, zResetUserPasswordBody, zSetSuperAdminBody, zUpdateUserBody } from "@/spx/api/iam/zod.gen";

import { Can } from "../components/access";
import { DataTable, usePagedQuery } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { dataOf, mutationHeaders, refreshIdentity } from "../iam/session";

type AccountAction = { action: "create" } | { action: "edit" | "reset" | "super"; user: ApiUser };
export function AccountsPage() {
  const t = useTranslations("Management");

  const state = usePagedQuery<ApiUser>(listUsersQueryKey({}), (query, signal) =>
    dataOf(listUsers({ query, signal, throwOnError: true })),
  );
  const [dialog, setDialog] = useState<AccountAction | null>(null);
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
        <AccountDialog
          dialog={dialog}
          onClose={() => setDialog(null)}
          onSaved={async () => {
            state.reload();
            await refreshIdentity();
          }}
        />
      )}
    </div>
  );
}

function AccountDialog({
  dialog,
  onClose,
  onSaved,
}: {
  dialog: AccountAction;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const t = useTranslations("Management");
  if (dialog.action === "create")
    return (
      <FormDialog
        title={t("createPlatformAccount")}
        schema={zCreateUserBody}
        defaultValues={{ username: "", display_name: "", email: "", password: "" }}
        onClose={onClose}
        onSubmit={async (body) => {
          await createUser({ body, headers: mutationHeaders, throwOnError: true });
          await onSaved();
        }}
      >
        {(form) => (
          <>
            <FormInput
              label={t("username")}
              registration={form.register("username")}
              error={form.formState.errors.username}
            />
            <FormInput
              label={t("displayName")}
              registration={form.register("display_name")}
              error={form.formState.errors.display_name}
            />
            <FormInput
              label={t("email")}
              type="email"
              registration={form.register("email")}
              error={form.formState.errors.email}
            />
            <FormInput
              label={t("password")}
              type="password"
              registration={form.register("password")}
              error={form.formState.errors.password}
            />
          </>
        )}
      </FormDialog>
    );
  if (dialog.action === "edit")
    return (
      <FormDialog
        title={t("editPlatformAccount")}
        schema={zUpdateUserBody}
        defaultValues={{
          display_name: dialog.user.display_name,
          email: dialog.user.email ?? "",
          status: dialog.user.status,
          auth_version: dialog.user.auth_version,
        }}
        onClose={onClose}
        onSubmit={async (body) => {
          await updateUser({ path: { id: dialog.user.id }, body, headers: mutationHeaders, throwOnError: true });
          await onSaved();
        }}
      >
        {(form) => (
          <>
            <FormInput
              label={t("displayName")}
              registration={form.register("display_name")}
              error={form.formState.errors.display_name}
            />
            <FormInput
              label={t("email")}
              type="email"
              registration={form.register("email")}
              error={form.formState.errors.email}
            />
            <Field>
              <FieldLabel>{t("status")}</FieldLabel>
              <NativeSelect {...form.register("status")}>
                <NativeSelectOption value="active">{t("active")}</NativeSelectOption>
                <NativeSelectOption value="disabled">{t("disabled")}</NativeSelectOption>
              </NativeSelect>
            </Field>
          </>
        )}
      </FormDialog>
    );
  if (dialog.action === "reset")
    return (
      <FormDialog
        title={t("resetPassword")}
        schema={zResetUserPasswordBody}
        defaultValues={{ password: "" }}
        onClose={onClose}
        onSubmit={async (body) => {
          await resetUserPassword({ path: { id: dialog.user.id }, body, headers: mutationHeaders, throwOnError: true });
          await onSaved();
        }}
      >
        {(form) => (
          <FormInput
            label={t("newPassword")}
            type="password"
            registration={form.register("password")}
            error={form.formState.errors.password}
            description={t("resetDescription")}
          />
        )}
      </FormDialog>
    );
  return (
    <FormDialog
      title={t("changeSuperAdmin")}
      schema={zSetSuperAdminBody}
      defaultValues={{ enabled: !dialog.user.is_super_admin, reason: "" }}
      onClose={onClose}
      onSubmit={async (body) => {
        await setSuperAdmin({ path: { id: dialog.user.id }, body, headers: mutationHeaders, throwOnError: true });
        await onSaved();
      }}
    >
      {(form) => (
        <FormInput
          label={t("reason")}
          type="textarea"
          registration={form.register("reason")}
          error={form.formState.errors.reason}
        />
      )}
    </FormDialog>
  );
}
