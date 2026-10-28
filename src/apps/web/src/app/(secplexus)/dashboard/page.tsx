import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import WorkbenchPage from "@/spx/pages/workbench";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("Metadata");
  return { title: t("dashboardTitle") };
}

export default function Page() {
  return <WorkbenchPage />;
}
