"use client";
import { type ReactNode, useId, useState } from "react";

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
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";

import { useAction } from "./data";

export interface FormField {
  name: string;
  label: string;
  type?: "text" | "password" | "email" | "textarea";
  required?: boolean;
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  placeholder?: string;
  description?: string;
  value?: string;
  options?: { value: string; label: string }[];
}
export function FormDialog({
  title,
  description,
  fields,
  children,
  onClose,
  onSubmit,
}: {
  title: string;
  description?: string;
  fields: FormField[];
  children?: ReactNode;
  onClose: () => void;
  onSubmit: (values: Record<string, string>) => Promise<unknown>;
}) {
  const perform = useAction();

  const t = useTranslations("Common");

  const [busy, setBusy] = useState(false);
  const id = useId();
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description ?? t("formDescription")}</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={async (event) => {
            event.preventDefault();
            const values = Object.fromEntries(
              [...new FormData(event.currentTarget)].map(([key, value]) => [key, String(value)]),
            );
            setBusy(true);
            const saved = await perform(() => onSubmit(values));
            setBusy(false);
            if (saved) onClose();
          }}
        >
          <fieldset disabled={busy} className="flex flex-col gap-5">
            <FieldGroup>
              {fields.map((field) => (
                <Field key={field.name}>
                  <FieldLabel htmlFor={`${id}-${field.name}`}>{field.label}</FieldLabel>
                  {field.options ? (
                    <NativeSelect
                      id={`${id}-${field.name}`}
                      name={field.name}
                      defaultValue={field.value || field.options[0].value}
                      required={field.required}
                    >
                      {field.options.map((option) => (
                        <NativeSelectOption key={option.value} value={option.value}>
                          {option.label}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  ) : field.type === "textarea" ? (
                    <Textarea
                      id={`${id}-${field.name}`}
                      name={field.name}
                      defaultValue={field.value}
                      required={field.required}
                      maxLength={field.maxLength}
                    />
                  ) : (
                    <Input
                      id={`${id}-${field.name}`}
                      name={field.name}
                      type={field.type || "text"}
                      defaultValue={field.value}
                      required={field.required}
                      minLength={field.minLength}
                      maxLength={field.maxLength}
                      pattern={field.pattern}
                      placeholder={field.placeholder}
                      autoComplete={field.type === "password" ? "new-password" : "off"}
                    />
                  )}
                  {field.description && <FieldDescription>{field.description}</FieldDescription>}
                </Field>
              ))}
              {children}
            </FieldGroup>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                {busy && <Spinner data-icon="inline-start" />}
                {t("confirm")}
              </Button>
            </DialogFooter>
          </fieldset>
        </form>
      </DialogContent>
    </Dialog>
  );
}
