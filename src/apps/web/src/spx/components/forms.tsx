"use client";
import { type ReactNode, useId } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useTranslations } from "next-intl";
import {
  type DefaultValues,
  type FieldValues,
  type FieldError as FormError,
  type UseFormRegisterReturn,
  type UseFormReturn,
  useForm,
  useFormState,
} from "react-hook-form";
import type { z } from "zod";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";

import { APIError } from "../api/client";
import { errorMessageKey } from "../iam/session";
import { requestReauth } from "./reauth";
import { SheetPanel } from "./sheet-panel";

// 表单外壳有对话框与抽屉两种形态：字段少的确认走对话框，
// 字段多的新建/编辑走右侧抽屉，正文独立滚动、底栏固定，绝不出现横向滚动。
export function FormDialog<TInput extends FieldValues, TOutput extends FieldValues>({
  title,
  description,
  schema,
  defaultValues,
  children,
  onClose,
  onSubmit,
  submitDisabled = false,
  variant = "dialog",
  confirmVariant = "default",
}: {
  title: string;
  description?: string;
  schema: z.ZodType<TOutput, TInput>;
  defaultValues: DefaultValues<TInput>;
  children?: (form: UseFormReturn<TInput, unknown, TOutput>) => ReactNode;
  submitDisabled?: boolean;
  onClose: () => void;
  onSubmit: (values: TOutput) => Promise<unknown>;
  variant?: "dialog" | "sheet";
  confirmVariant?: "default" | "destructive";
}) {
  const t = useTranslations("Common");
  const errors = useTranslations("Errors");
  const descriptionID = useId();
  const formID = useId();
  const form = useForm<TInput, unknown, TOutput>({ resolver: zodResolver(schema), defaultValues });
  const businessState = useFormState({ control: form.control });
  const busy = businessState.isSubmitting;

  // save 在需要近期密码确认时先走全局确认框，确认成功后重放一次提交。
  // 表单本身不承载确认字段：同一个确认框覆盖抽屉与对话框两种形态。
  const save = async (values: TOutput) => {
    try {
      await onSubmit(values);
    } catch (error) {
      if (!(error instanceof APIError) || error.code !== "iam.recent_auth_required") throw error;
      if (!(await requestReauth())) throw error;
      await onSubmit(values);
    }
  };

  const close = () => {
    if (busy) return;
    form.reset();
    onClose();
  };

  const body = (
    <form
      id={formID}
      noValidate
      onSubmit={form.handleSubmit(async (values) => {
        form.clearErrors("root");
        try {
          await save(values);
          form.reset();
          onClose();
        } catch (error) {
          if (!(error instanceof DOMException && error.name === "AbortError"))
            form.setError("root", { message: errors(errorMessageKey(error)) });
        }
      })}
    >
      <fieldset disabled={busy} className="flex flex-col gap-5">
        <FieldGroup>{children?.({ ...form, formState: businessState })}</FieldGroup>
        {businessState.errors.root && <FieldError>{businessState.errors.root.message}</FieldError>}
      </fieldset>
    </form>
  );

  const footer = (
    <>
      <Button type="button" variant="outline" onClick={close} disabled={busy}>
        {t("cancel")}
      </Button>
      <Button type="submit" form={formID} variant={confirmVariant} disabled={busy || submitDisabled}>
        {busy && <Spinner data-icon="inline-start" />}
        {t("confirm")}
      </Button>
    </>
  );

  if (variant === "sheet")
    return (
      <SheetPanel title={title} description={description} footer={footer} onClose={close}>
        <div className="px-6 py-5">{body}</div>
      </SheetPanel>
    );

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        aria-describedby={description ? descriptionID : undefined}
        onEscapeKeyDown={(event) => {
          // Base UI 的候选弹层与 Radix Dialog 分别处理 Escape，先让候选框关闭。
          const target = event.target;
          if (
            target instanceof HTMLElement &&
            target.getAttribute("role") === "combobox" &&
            target.getAttribute("aria-expanded") === "true"
          )
            event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription id={descriptionID}>{description}</DialogDescription>}
        </DialogHeader>
        {body}
        <DialogFooter>{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** FormInput 复用现有 Field 和输入样式，错误文案由 next-intl 提供。 */
export function FormInput({
  label,
  registration,
  error,
  errorText,
  type = "text",
  description,
  placeholder,
}: {
  label: string;
  registration: UseFormRegisterReturn;
  error?: FormError;
  errorText?: string;
  type?: "text" | "password" | "email" | "textarea";
  description?: string;
  placeholder?: string;
}) {
  const id = useId();
  const errors = useTranslations("Errors");
  return (
    <Field data-invalid={!!error}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {type === "textarea" ? (
        <Textarea id={id} {...registration} aria-invalid={!!error} />
      ) : (
        <Input
          id={id}
          {...registration}
          type={type}
          placeholder={placeholder}
          autoComplete={type === "password" ? "new-password" : "off"}
          aria-invalid={!!error}
        />
      )}
      {description && <FieldDescription>{description}</FieldDescription>}
      {error && <FieldError>{errorText ?? errors("invalidRequest")}</FieldError>}
    </Field>
  );
}
