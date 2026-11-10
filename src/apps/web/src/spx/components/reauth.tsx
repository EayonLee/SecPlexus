"use client";
import { useEffect, useState } from "react";

import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";

import { errorMessageKey, reauthenticate } from "../iam/session";

// 高风险操作（改权限、重置密码、改用户/角色）要求近期密码确认。
// 任一动作收到 iam.recent_auth_required 时调用 requestReauth()，确认后由调用方重放。
let pending: ((confirmed: boolean) => void) | null = null;
const listeners = new Set<() => void>();

/** requestReauth 弹出密码确认；返回是否确认成功，多个调用共享同一个对话框。 */
export function requestReauth(): Promise<boolean> {
  return new Promise((resolve) => {
    pending?.(false);
    pending = resolve;
    for (const listener of listeners) listener();
  });
}

/** ReauthGate 是密码确认对话框的唯一实例，挂在应用外壳里。 */
export function ReauthGate() {
  const account = useTranslations("Account");
  const common = useTranslations("Common");
  const errors = useTranslations("Errors");
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const sync = () => setOpen(pending !== null);
    listeners.add(sync);
    return () => {
      listeners.delete(sync);
    };
  }, []);

  const settle = (confirmed: boolean) => {
    const resolve = pending;
    pending = null;
    setOpen(false);
    setPassword("");
    setMessage(null);
    resolve?.(confirmed);
  };

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setBusy(true);
    setMessage(null);
    try {
      await reauthenticate(password);
      settle(true);
    } catch (failure) {
      setMessage(errors(errorMessageKey(failure)));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) settle(false);
      }}
    >
      <DialogContent showCloseButton={false}>
        <form className="flex flex-col gap-4" onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{account("confirmCurrentPassword")}</DialogTitle>
            <DialogDescription>{errors("recentAuthRequired")}</DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor="spx-reauth-password">{account("currentPassword")}</FieldLabel>
            <Input
              id="spx-reauth-password"
              type="password"
              autoComplete="current-password"
              autoFocus
              disabled={busy}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
            {message && <FieldError>{message}</FieldError>}
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={busy} onClick={() => settle(false)}>
              {common("cancel")}
            </Button>
            <Button type="submit" disabled={busy || password === ""}>
              {busy ? <Spinner /> : common("confirm")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
