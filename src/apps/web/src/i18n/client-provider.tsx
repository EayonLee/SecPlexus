"use client";

import type { ReactNode } from "react";

import { NextIntlClientProvider, useLocale } from "next-intl";

import { messageFallback } from "./config";

// Function-valued next-intl options cannot pass through the Server Component boundary.
export function ClientMessages({ children }: { children: ReactNode }) {
  return (
    <NextIntlClientProvider locale={useLocale()} getMessageFallback={messageFallback}>
      {children}
    </NextIntlClientProvider>
  );
}
