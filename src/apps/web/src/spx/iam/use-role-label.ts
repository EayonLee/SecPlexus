import { useTranslations } from "next-intl";

import type { components } from "@/auth/api.gen";

export function useRoleLabel() {
  const t = useTranslations("Management");
  const systemRoles: Record<string, string> = { org_admin: t("organizationAdmin"), member: t("organizationMember") };
  return (role: Pick<components["schemas"]["APIRole"], "kind" | "code" | "name">) =>
    role.kind === "system" ? (systemRoles[role.code] ?? role.name) : role.name;
}
