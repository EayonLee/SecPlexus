import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { LoginPage } from "@/spx/pages/login";
export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("Metadata");
  return { title: t("loginTitle") };
}
export default function Page() {
  return <LoginPage />;
}
