"use server";

import { cookies } from "next/headers";

import { hasLocale } from "next-intl";

import { type Locale, localeCookie, locales } from "./config";

export async function setLocale(locale: Locale) {
  if (!hasLocale(locales, locale)) throw new Error("Unsupported locale");
  (await cookies()).set(localeCookie, locale, { path: "/", maxAge: 31536000, sameSite: "lax", httpOnly: true });
}
