"use client";
import { useState } from "react";

import { useMutation } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { Controller } from "react-hook-form";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { createUser, listUsers, resetUserPassword, updateUser } from "@/spx/api/iam/management";
import type { ApiUser } from "@/spx/api/iam/schemas";
import { zCreateUserBody, zResetUserPasswordBody, zUpdateUserBody } from "@/spx/api/iam/schemas";

import { Can } from "../components/access";
import { DataTable, usePagedQuery } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { StatusBadge } from "../components/status-badge";
import { managementChanged } from "../iam/management";

type AccountAction = { action: "create" } | { action: "edit" | "reset"; user: ApiUser };
export function AccountsPage() {
  const t = useTranslations("Management");

  const state = usePagedQuery<ApiUser>(["iam", "users"], listUsers);
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
            cell: (user) => (
              <StatusBadge status={user.status}>{user.status === "active" ? t("active") : t("disabled")}</StatusBadge>
            ),
          },
          { title: t("platformRole"), cell: (user) => (user.is_super_admin ? t("superAdmin") : t("regularUser")) },
          {
            title: t("action"),
            cell: (user) =>
              user.is_super_admin ? (
                <StatusBadge status="system">{t("configurationManaged")}</StatusBadge>
              ) : (
                <div className="flex gap-2">
                  <Can action={permissions.IAMPlatformUsersManage}>
                    <Button variant="ghost" onClick={() => setDialog({ action: "edit", user })}>
                      {t("edit")}
                    </Button>
                    <Button variant="ghost" onClick={() => setDialog({ action: "reset", user })}>
                      {t("resetPassword")}
                    </Button>
                  </Can>
                </div>
              ),
          },
        ]}
      />
      {dialog && <AccountDialog dialog={dialog} onClose={() => setDialog(null)} onSaved={managementChanged} />}
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
  const edit = useMutation({ mutationFn: updateUser, onSuccess: managementChanged });
  if (dialog.action === "create")
    return (
      <FormDialog
        title={t("createPlatformAccount")}
        schema={zCreateUserBody}
        defaultValues={{ username: "", display_name: "", email: "", password: "" }}
        onClose={onClose}
        onSubmit={async (body) => {
          await createUser(body);
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
          await edit.mutateAsync({ id: dialog.user.id, body });
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
              <Controller
                control={form.control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange} name={field.name}>
                    <SelectTrigger id="accounts-status" aria-label={t("status")} onBlur={field.onBlur} ref={field.ref}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="active">{t("active")}</SelectItem>
                        <SelectItem value="disabled">{t("disabled")}</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                )}
              />
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
          await resetUserPassword({ id: dialog.user.id, body });
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
  return null;
}
