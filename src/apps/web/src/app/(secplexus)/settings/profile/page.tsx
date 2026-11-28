import { getTranslations } from "next-intl/server";

import { ProfilePage } from "@/spx/pages/profile";

export async function generateMetadata() {
  const t = await getTranslations("Navigation");
  return { title: t("profile") };
}

export default function Page() {
  return <ProfilePage />;
}
