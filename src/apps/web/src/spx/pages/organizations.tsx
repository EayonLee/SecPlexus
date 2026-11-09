"use client";
import { useState } from "react";

import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { Controller } from "react-hook-form";
import { Building2Icon, PlusIcon } from "lucide-react";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { createOrganization, listOrganizationTree, updateOrganization } from "@/spx/api/iam/management";
import type { ApiOrganization } from "@/spx/api/iam/schemas";
import { zCreateOrganizationBody, zUpdateOrganizationBody } from "@/spx/api/iam/schemas";

import { Can } from "../components/access";
import { FormDialog, FormInput } from "../components/forms";
import { OrganizationTree, descendantIDs } from "../components/organization-tree";
import { OrganizationSplitPane } from "../components/organization-split-pane";
import { UserPicker } from "../components/user-picker";
import { managementChanged } from "../iam/management";

// subtreeOf 返回选定组织及其全部后代；根节点为空时视为全部组织。
// 组织树接口一次返回全部节点，因此这里在客户端展开，不再额外请求。
function subtreeOf(items: ApiOrganization[], rootID: string | undefined) {
  if (!rootID) return items;

  const children = new Map<string, ApiOrganization[]>();
  for (const item of items) {
    const key = item.parent_id ?? "";
    children.set(key, [...(children.get(key) ?? []), item]);
  }

  const collected: ApiOrganization[] = [];
  const walk = (id: string) => {
    const node = items.find((item) => item.id === id);
    if (!node) return;
    collected.push(node);
    for (const child of children.get(id) ?? []) walk(child.id);
  };
  walk(rootID);

  return collected;
}

export function OrganizationsPage() {
  const t = useTranslations("Management");

  const tree = useQuery({
    queryKey: ["iam", "organizations", "tree"],
    queryFn: ({ signal }) => listOrganizationTree(signal),
  });
  const items = tree.data?.items ?? [];
  const truncated = tree.data?.truncated ?? false;

  const [dialog, setDialog] = useState<"create" | ApiOrganization>();

  const create = useMutation({ mutationFn: createOrganization, onSuccess: managementChanged });
  const edit = useMutation({ mutationFn: updateOrganization, onSuccess: managementChanged });

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h2 className="text-base font-medium">{t("organizationManage")}</h2>
          {truncated && <p className="text-muted-foreground text-sm">{t("truncatedOrganizations")}</p>}
        </div>
        <div className="flex shrink-0 gap-2">
          <Can action={permissions.IAMPlatformOrganizationsManage}>
            <Button onClick={() => setDialog("create")}>
              <PlusIcon data-icon="inline-start" />
              {t("createOrg")}
            </Button>
          </Can>
          <Button variant="outline" onClick={() => void tree.refetch()} disabled={tree.isFetching}>
            {t("refresh")}
          </Button>
        </div>
      </div>

      {tree.isError && <p role="alert">{t("loadOrganizationFailed")}</p>}

      {!tree.isPending && items.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Building2Icon aria-hidden />
            </EmptyMedia>
            <EmptyTitle>{t("organizationTree")}</EmptyTitle>
            <EmptyDescription>{t("createOrg")}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <OrganizationSplitPane
          items={items}
          loading={tree.isPending}
          heading={t("organizationTree")}
          description={t("organizationTreeDescription")}
        >
          {(selected) => {
            const rows = subtreeOf(items, selected?.id);
            return (
              <>
                <div className="text-muted-foreground flex items-center justify-between text-sm">
                  <span>{selected ? selected.name : t("rootOrganization")}</span>
                  <span>{t("organizationCount", { count: rows.length })}</span>
                </div>
                <div className="overflow-hidden rounded-md border">
                  <Table>
                    <TableHeader className="bg-muted">
                      <TableRow>
                        <TableHead>{t("orgName")}</TableHead>
                        <TableHead>{t("orgCode")}</TableHead>
                        <TableHead className="w-0">{t("action")}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {rows.length ? (
                        rows.map((org) => (
                          <TableRow key={org.id}>
                            <TableCell>{org.name}</TableCell>
                            <TableCell className="text-muted-foreground">{org.slug}</TableCell>
                            <TableCell>
                              <div className="flex justify-end gap-1">
                                <Button variant="ghost" size="sm" onClick={() => setDialog(org)}>
                                  {t("edit")}
                                </Button>
                                <Button variant="ghost" size="sm" onClick={() => setDialog(org)}>
                                  {t("delete")}
                                </Button>
                              </div>
                            </TableCell>
                          </TableRow>
                        ))
                      ) : (
                        <TableRow>
                          <TableCell colSpan={3}>
                            <Empty>
                              <EmptyHeader>
                                <EmptyTitle>{t("organizationEmpty")}</EmptyTitle>
                                <EmptyDescription>{t("organizationEmptyDescription")}</EmptyDescription>
                              </EmptyHeader>
                            </Empty>
                          </TableCell>
                        </TableRow>
                      )}
                    </TableBody>
                  </Table>
                </div>
              </>
            );
          }}
        </OrganizationSplitPane>
      )}

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
              <Field>
                <FieldLabel>{t("parentOrganization")}</FieldLabel>
                {items.length ? (
                  <div className="bg-muted/30 rounded-md border p-2">
                    <OrganizationTree
                      items={items}
                      selectedID={form.watch("parent_id")}
                      variant="selectable"
                      onSelect={(node) => form.setValue("parent_id", node.id, { shouldDirty: true })}
                    />
                  </div>
                ) : (
                  <p className="text-muted-foreground text-sm">{t("rootOrganization")}</p>
                )}
              </Field>
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
          defaultValues={{
            name: dialog.name,
            authz_epoch: dialog.authz_epoch,
            parent_id: dialog.parent_id ?? undefined,
          }}
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
                <FieldLabel>{t("changeParent")}</FieldLabel>
                <div className="bg-muted/30 rounded-md border p-2">
                  <OrganizationTree
                    items={items}
                    selectedID={form.watch("parent_id")}
                    variant="selectable"
                    disabledIDs={descendantIDs(items, dialog.id)}
                    onSelect={(node) => form.setValue("parent_id", node.id, { shouldDirty: true })}
                  />
                </div>
              </Field>
            </>
          )}
        </FormDialog>
      )}
    </div>
  );
}
