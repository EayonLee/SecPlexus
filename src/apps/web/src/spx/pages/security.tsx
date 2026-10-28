"use client";
import { useEffect, useState } from "react";

import { useRouter } from "next/navigation";

import { useFormatter, useTranslations } from "next-intl";
import { toast } from "sonner";

import type { components } from "@/auth/api.gen";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import { useAction } from "../components/data";
import { FormDialog } from "../components/forms";
import { api, changePassword, reauthenticate, signOut, useIdentity } from "../iam/session";

export function SecurityPage() {
  const perform = useAction();

  const t = useTranslations("Account");
  const format = useFormatter();

  const auth = useIdentity();
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [sessions, setSessions] = useState<components["schemas"]["APISession"][]>([]);
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: 手动刷新和设备撤销后重新读取会话列表。
  useEffect(() => {
    const controller = new AbortController();
    void perform(async () => {
      const data = await api<{ items: components["schemas"]["APISession"][] }>("/iam/me/sessions", {
        signal: controller.signal,
      });
      if (!controller.signal.aborted) setSessions(data.items);
    });
    return () => controller.abort();
  }, [revision]);
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
            onSubmit={async (event) => {
              event.preventDefault();
              const values = new FormData(event.currentTarget);
              const form = event.currentTarget;
              if (values.get("new_password") !== values.get("confirm_password")) {
                toast.error(t("passwordMismatch"));
                return;
              }
              setBusy(true);
              await perform(async () => {
                await changePassword(String(values.get("old_password")), String(values.get("new_password")));
                form.reset();
                toast.success(t("passwordChanged"));
                router.replace("/login");
              });
              setBusy(false);
            }}
          >
            <fieldset disabled={busy}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="old-password">{t("currentPassword")}</FieldLabel>
                  <Input
                    id="old-password"
                    name="old_password"
                    type="password"
                    autoComplete="current-password"
                    required
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="new-password">{t("newPassword")}</FieldLabel>
                  <Input
                    id="new-password"
                    name="new_password"
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
                    name="confirm_password"
                    type="password"
                    autoComplete="new-password"
                    required
                  />
                </Field>
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
            <Button variant="outline" onClick={() => setRevision((value) => value + 1)}>
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
                          await api(`/iam/me/sessions/${session.id}`, { method: "DELETE" });
                          if (session.current) {
                            window.location.assign("/login");
                          } else setRevision((value) => value + 1);
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
          fields={[{ name: "password", label: t("confirmCurrentPassword"), type: "password", required: true }]}
          onClose={() => setConfirming(false)}
          onSubmit={async (values) => {
            await reauthenticate(values.password);
            setRevision((value) => value + 1);
            toast.success(t("verified"));
          }}
        />
      )}
    </div>
  );
}
