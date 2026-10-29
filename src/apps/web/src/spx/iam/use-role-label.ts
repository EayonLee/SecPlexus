import { useTranslations } from "next-intl";

import type { ApiRole } from "@/spx/api/iam/schemas";

export function useRoleLabel() {
  const t = useTranslations("Management");
  const systemRoles: Record<string, string> = { org_admin: t("organizationAdmin"), member: t("organizationMember") };
  return (role: Pick<ApiRole, "kind" | "code" | "name">) =>
    role.kind === "system" ? (systemRoles[role.code] ?? role.name) : role.name;
}
