"use client";
import { useState } from "react";

import { useRouter } from "next/navigation";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { useFormatter, useTranslations } from "next-intl";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { listSessionsQueryKey } from "@/spx/api/iam/@tanstack/react-query.gen";
import { listSessions, revokeSession } from "@/spx/api/iam/sdk.gen";
import { zChangePasswordBody, zReauthenticateBody } from "@/spx/api/iam/zod.gen";

import { useAction } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { changePassword, dataOf, mutationHeaders, reauthenticate, signOut, useIdentity } from "../iam/session";

export function SecurityPage() {
  const perform = useAction();

  const t = useTranslations("Account");
  const errors = useTranslations("Errors");
  const format = useFormatter();

  const auth = useIdentity();
  const router = useRouter();
  const passwordSchema = zChangePasswordBody
    .extend({ confirm_password: z.string() })
    .refine((value) => value.new_password === value.confirm_password, {
      path: ["confirm_password"],
      message: t("passwordMismatch"),
    });
  const passwordForm = useForm<z.infer<typeof passwordSchema>>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { old_password: "", new_password: "", confirm_password: "" },
  });
  const busy = passwordForm.formState.isSubmitting;
  const [confirming, setConfirming] = useState(false);
  const devices = useQuery({
    queryKey: [...listSessionsQueryKey(), auth.principal?.session_id, auth.principal?.auth_version],
    queryFn: ({ signal }) => dataOf(listSessions({ signal, throwOnError: true })),
  });
  const sessions = devices.data?.items ?? [];
  return (
    <div className="flex w-full flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("security")}</CardTitle>
          <CardDescription>
            {auth.principal?.display_name} · {auth.principal?.username}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-6">
          <form
            className="max-w-lg"
            noValidate
            onSubmit={passwordForm.handleSubmit(async (values) => {
              await perform(async () => {
                await changePassword(values.old_password, values.new_password);
                passwordForm.reset();
                toast.success(t("passwordChanged"));
                router.replace("/login");
              });
            })}
          >
            <fieldset disabled={busy}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="old-password">{t("currentPassword")}</FieldLabel>
                  <Input
                    id="old-password"
                    {...passwordForm.register("old_password")}
                    type="password"
                    autoComplete="current-password"
                    required
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="new-password">{t("newPassword")}</FieldLabel>
                  <Input
                    id="new-password"
                    {...passwordForm.register("new_password")}
                    type="password"
                    autoComplete="new-password"
                    minLength={8}
                    maxLength={128}
                    required
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="confirm-password">{t("confirmPassword")}</FieldLabel>
                  <Input
                    id="confirm-password"
                    {...passwordForm.register("confirm_password")}
                    type="password"
                    autoComplete="new-password"
                    required
                  />
                </Field>
                {Object.keys(passwordForm.formState.errors).length > 0 && (
                  <FieldError>
                    {passwordForm.formState.errors.confirm_password?.message ?? errors("invalidRequest")}
                  </FieldError>
                )}
                <Field orientation="horizontal">
                  <Button type="submit" disabled={busy}>
                    {t("changeAndSignOut")}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() =>
                      void perform(async () => {
                        await signOut();
                        router.replace("/login");
                      })
                    }
                  >
                    {t("signOut")}
                  </Button>
                </Field>
              </FieldGroup>
            </fieldset>
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("devices")}</CardTitle>
          <CardDescription>{t("devicesDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => setConfirming(true)}>{t("reauthenticate")}</Button>
            <Button variant="outline" onClick={() => void devices.refetch()}>
              {t("refresh")}
            </Button>
            <Button
              variant="destructive"
              onClick={() =>
                void perform(async () => {
                  await signOut(true);
                  router.replace("/login");
                })
              }
            >
              {t("signOutAll")}
            </Button>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                {[t("device"), "IP", t("lastSeen"), t("status"), t("actions")].map((title) => (
                  <TableHead key={title}>{title}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {sessions.map((session) => (
                <TableRow key={session.id}>
                  <TableCell className="max-w-72 truncate">{session.user_agent}</TableCell>
                  <TableCell>{session.client_ip}</TableCell>
                  <TableCell>
                    {format.dateTime(new Date(session.last_seen_at), { dateStyle: "medium", timeStyle: "medium" })}
                  </TableCell>
                  <TableCell>{session.current ? t("currentDevice") : t("otherDevice")}</TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      onClick={() =>
                        void perform(async () => {
                          await revokeSession({
                            throwOnError: true,
                            path: { id: session.id },
                            headers: mutationHeaders,
                          });
                          if (session.current) {
                            window.location.assign("/login");
                          } else void devices.refetch();
                        })
                      }
                    >
                      {t("revoke")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      {confirming && (
        <FormDialog
          title={t("confirmCurrentPassword")}
          schema={zReauthenticateBody}
          defaultValues={{ password: "" }}
          onClose={() => setConfirming(false)}
          onSubmit={async (values) => {
            await reauthenticate(values.password);
            void devices.refetch();
            toast.success(t("verified"));
          }}
        >
          {(form) => (
            <FormInput
              label={t("confirmCurrentPassword")}
              type="password"
              registration={form.register("password")}
              error={form.formState.errors.password}
            />
          )}
        </FormDialog>
      )}
    </div>
  );
}
