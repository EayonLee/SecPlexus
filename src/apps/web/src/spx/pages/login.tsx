"use client";
import { useEffect, useState } from "react";

import { useRouter } from "next/navigation";

import { Command } from "lucide-react";
import { useTranslations } from "next-intl";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";

import { LanguageSwitcher } from "../components/language-switcher";
import { errorMessageKey, initializeSession, signIn, useIdentity } from "../iam/session";

export function LoginPage() {
  const t = useTranslations("Auth");
  const errors = useTranslations("Errors");

  const router = useRouter();
  const auth = useIdentity();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ReturnType<typeof errorMessageKey> | "">("");
  const [username, setUsername] = useState("");
  const [remember, setRemember] = useState(false);
  useEffect(() => {
    const remembered = localStorage.getItem("spx-remembered-username") || "";
    setUsername(remembered);
    setRemember(!!remembered);
    void initializeSession();
  }, []);
  useEffect(() => {
    if (auth.status === "authenticated") router.replace("/dashboard");
  }, [auth.status, router]);
  return (
    <div className="relative flex h-dvh">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher />
      </div>
      <div className="bg-primary hidden lg:block lg:w-1/3">
        <div className="flex h-full flex-col items-center justify-center p-12 text-center">
          <div className="flex flex-col gap-6">
            <Command className="text-primary-foreground mx-auto size-12" />
            <div className="flex flex-col gap-2">
              <h1 className="text-primary-foreground text-5xl font-light">SecPlexus</h1>
              <p className="text-primary-foreground/80 text-xl">{t("continue")}</p>
            </div>
          </div>
        </div>
      </div>
      <div className="bg-background flex w-full items-center justify-center p-8 lg:w-2/3">
        <div className="flex w-full max-w-md flex-col gap-10 py-24 lg:py-32">
          <div className="flex flex-col gap-4 text-center">
            <h2 className="font-medium tracking-tight">{t("signIn")}</h2>
            <p className="text-muted-foreground mx-auto max-w-xl">{t("description")}</p>
          </div>
          {auth.status === "unavailable" && (
            <Alert variant="destructive">
              <AlertDescription>
                {t("unavailable")}
                <Button variant="link" onClick={() => void initializeSession()}>
                  {t("reconnect")}
                </Button>
              </AlertDescription>
            </Alert>
          )}
          <form
            method="post"
            className="flex flex-col gap-4"
            onSubmit={async (event) => {
              event.preventDefault();
              const form = new FormData(event.currentTarget);
              setBusy(true);
              setError("");
              try {
                await signIn(username, String(form.get("password")));
                if (remember) localStorage.setItem("spx-remembered-username", username);
                else localStorage.removeItem("spx-remembered-username");
              } catch (failure) {
                setError(errorMessageKey(failure));
              } finally {
                setBusy(false);
              }
            }}
          >
            <fieldset
              disabled={busy || auth.status === "unknown" || auth.status === "unavailable"}
              className="flex flex-col gap-4"
            >
              <FieldGroup className="gap-4">
                <Field className="gap-1.5">
                  <FieldLabel htmlFor="username">{t("username")}</FieldLabel>
                  <Input
                    id="username"
                    name="username"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                    autoComplete="username"
                    required
                  />
                </Field>
                <Field className="gap-1.5">
                  <FieldLabel htmlFor="password">{t("password")}</FieldLabel>
                  <Input id="password" name="password" type="password" autoComplete="current-password" required />
                </Field>
                <Field orientation="horizontal">
                  <Checkbox
                    id="remember"
                    checked={remember}
                    onCheckedChange={(checked) => setRemember(checked === true)}
                  />
                  <FieldLabel htmlFor="remember">{t("remember")}</FieldLabel>
                </Field>
                {error && <FieldError>{errors(error)}</FieldError>}
              </FieldGroup>
              <Button type="submit" disabled={busy}>
                {busy && <Spinner data-icon="inline-start" />}
                {t("signIn")}
              </Button>
            </fieldset>
          </form>
        </div>
      </div>
    </div>
  );
}
