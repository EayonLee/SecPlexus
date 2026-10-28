"use client";
import { useState } from "react";

import { useTranslations } from "next-intl";
import { Controller } from "react-hook-form";
import { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldDescription, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { listMembersQueryKey } from "@/spx/api/iam/@tanstack/react-query.gen";
import {
  createMember,
  deleteMember,
  listMembers,
  listRoles,
  setMemberRoles,
  updateMember,
} from "@/spx/api/iam/sdk.gen";
import type { ApiMember, ApiRole } from "@/spx/api/iam/types.gen";
import { zCreateMemberBody, zSetMemberRolesBody, zUpdateMemberBody } from "@/spx/api/iam/zod.gen";

import { Can, useCan } from "../components/access";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { UserPicker } from "../components/user-picker";
import { dataOf, mutationHeaders, refreshIdentity, useIdentity } from "../iam/session";
import { useRoleLabel } from "../iam/use-role-label";

export function MembersPage() {
  const roleLabel = useRoleLabel();
  const perform = useAction();

  const t = useTranslations("Management");

  const auth = useIdentity();
  const org = auth.org_id ?? "";
  const state = usePagedQuery<ApiMember>(listMembersQueryKey({ path: { org_id: org } }), (query, signal) =>
    dataOf(listMembers({ path: { org_id: org }, query, signal, throwOnError: true })),
  );
  const [creating, setCreating] = useState(false);
  const [existing, setExisting] = useState(false);
  const [assigning, setAssigning] = useState<ApiMember | undefined>(undefined);
  const [removing, setRemoving] = useState<ApiMember | undefined>(undefined);
  const [roles, setRoles] = useState<ApiRole[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const canAssign = useCan(permissions.IAMRolesAssign);
  const canReadRoles = useCan(permissions.IAMRolesRead);
  const version = auth.organizations.find((entry) => entry.id === org)?.authz_version;
  async function changed() {
    await refreshIdentity(org);
    state.reload();
  }
  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        <Can action={permissions.IAMMembersManage}>
          <Button
            onClick={() => {
              setExisting(false);
              setCreating(true);
            }}
          >
            {t("addMember")}
          </Button>
        </Can>
        <Button variant="outline" onClick={() => void perform(changed)}>
          {t("refresh")}
        </Button>
      </div>
      <DataTable
        state={state}
        columns={[
          { title: t("username"), cell: (member) => member.username },
          { title: t("displayName"), cell: (member) => member.display_name },
          { title: t("status"), cell: (member) => (member.status === "active" ? t("active") : t("disabled")) },
          {
            title: t("action"),
            cell: (member) => (
              <div className="flex gap-2">
                {canAssign && canReadRoles && (
                  <Button
                    variant="ghost"
                    onClick={() =>
                      void perform(async () => {
                        const data = await dataOf(
                          listRoles({ throwOnError: true, path: { org_id: org }, query: { limit: 100 } }),
                        );
                        setRoles(data.items);
                        setSelected(member.role_ids ?? []);
                        await refreshIdentity(org);
                        setAssigning(member);
                      })
                    }
                  >
                    {t("assignRoles")}
                  </Button>
                )}
                <Can action={permissions.IAMMembersManage}>
                  <Button
                    variant="ghost"
                    onClick={() =>
                      void perform(async () => {
                        await updateMember({
                          throwOnError: true,
                          path: { org_id: org, id: member.id },
                          headers: mutationHeaders,
                          body: zUpdateMemberBody.parse({
                            status: member.status === "active" ? "disabled" : "active",
                            authz_version: version,
                          }),
                        });
                        await changed();
                      })
                    }
                  >
                    {member.status === "active" ? t("disabled") : t("active")}
                  </Button>
                  <Button variant="ghost" onClick={() => setRemoving(member)}>
                    {t("remove")}
                  </Button>
                </Can>
              </div>
            ),
          },
        ]}
      />
      {creating && (
        existing ? (
            <FormDialog
              key="existing"
              title={t("addOrgMember")}
              schema={zCreateMemberBody.pick({ user_id: true }).required()}
              defaultValues={{ user_id: "" }}
              onClose={() => setCreating(false)}
              onSubmit={async (body) => {
                await createMember({ path: { org_id: org }, body, headers: mutationHeaders, throwOnError: true });
                await changed();
              }}
            >
              {(form) => (
                <>
                  {auth.principal?.is_super_admin && (
                    <Field orientation="horizontal">
                      <Checkbox
                        id="existing-member"
                        checked={existing}
                        onCheckedChange={(checked) => setExisting(checked === true)}
                      />
                      <FieldLabel htmlFor="existing-member">{t("addExistingAccount")}</FieldLabel>
                    </Field>
                  )}
                  <Field>
                    <FieldLabel>{t("existingAccount")}</FieldLabel>
                    <Controller
                      control={form.control}
                      name="user_id"
                      render={({ field }) => <UserPicker value={field.value} onValueChange={field.onChange} />}
                    />
                    <FieldDescription>{t("existingAccountDescription")}</FieldDescription>
                  </Field>
                </>
              )}
            </FormDialog>
          ) : (
            <FormDialog
              key="new"
              title={t("addOrgMember")}
              schema={zCreateMemberBody.pick({ username: true, display_name: true, password: true }).required()}
              defaultValues={{ username: "", display_name: "", password: "" }}
              onClose={() => setCreating(false)}
              onSubmit={async (body) => {
                await createMember({ path: { org_id: org }, body, headers: mutationHeaders, throwOnError: true });
                await changed();
              }}
            >
              {(form) => (
                <>
                  {auth.principal?.is_super_admin && (
                    <Field orientation="horizontal">
                      <Checkbox
                        id="existing-member"
                        checked={existing}
                        onCheckedChange={(checked) => setExisting(checked === true)}
                      />
                      <FieldLabel htmlFor="existing-member">{t("addExistingAccount")}</FieldLabel>
                    </Field>
                  )}
                  <FormInput
                    label={t("newUsername")}
                    registration={form.register("username")}
                    error={form.formState.errors.username}
                  />
                  <FormInput
                    label={t("displayName")}
                    registration={form.register("display_name")}
                    error={form.formState.errors.display_name}
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
          )
      )}
      {assigning && (
        <FormDialog
          title={t("assignMemberRoles", { name: assigning.display_name })}
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setAssigning(undefined)}
          onSubmit={async () => {
            await setMemberRoles({
              throwOnError: true,
              path: { org_id: org, id: assigning.id },
              headers: mutationHeaders,
              body: zSetMemberRolesBody.parse({ role_ids: selected, authz_version: version }),
            });
            await changed();
          }}
        >
          {() => (
            <FieldSet>
              <FieldLegend>{t("memberRoles")}</FieldLegend>
              {roles.map((role) => (
                <Field key={role.id} orientation="horizontal">
                  <Checkbox
                    id={`member-role-${role.id}`}
                    checked={selected.includes(role.id)}
                    disabled={role.status !== "active"}
                    onCheckedChange={(checked) =>
                      setSelected((current) =>
                        checked === true ? [...current, role.id] : current.filter((id) => id !== role.id),
                      )
                    }
                  />
                  <FieldLabel htmlFor={`member-role-${role.id}`}>{roleLabel(role)}</FieldLabel>
                </Field>
              ))}
            </FieldSet>
          )}
        </FormDialog>
      )}
      {removing && (
        <FormDialog
          title={t("removeMember")}
          description={t("removeMemberDescription", { name: removing.display_name })}
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setRemoving(undefined)}
          onSubmit={async () => {
            await deleteMember({
              throwOnError: true,
              path: { org_id: org, id: removing.id },
              query: { authz_version: version ?? "" },
              headers: mutationHeaders,
            });
            await changed();
          }}
        />
      )}
    </div>
  );
}
