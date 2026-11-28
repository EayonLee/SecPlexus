import { useTranslations } from "next-intl";

import { type PermissionKey, permissions } from "@/auth/permissions.gen";

const descriptionKeys = {
  [permissions.ConsoleContextRead]: "contextRead",
  [permissions.IAMAuditRead]: "auditRead",
  [permissions.IAMAuditWrite]: "auditWrite",
  [permissions.IAMPermissionsRead]: "permissionsRead",
  [permissions.IAMRolesManage]: "rolesManage",
  [permissions.IAMRolesRead]: "rolesRead",
  [permissions.IAMUsersManage]: "usersManage",
  [permissions.IAMUsersRead]: "usersRead",
  [permissions.IAMUsersReset]: "usersReset",
  [permissions.IAMUsersRoles]: "usersRoles",
  [permissions.IngestEventsRead]: "ingestEventsRead",
  [permissions.IngestInputRead]: "inputRead",
  [permissions.IngestInputWrite]: "inputWrite",
  [permissions.IngestParsingRuleRead]: "mappingRead",
  [permissions.IngestParsingRuleWrite]: "mappingWrite",
} as const satisfies Record<PermissionKey, string>;

function isKnownPermission(key: string): key is PermissionKey {
  return Object.hasOwn(descriptionKeys, key);
}

export function usePermissionDescription() {
  const t = useTranslations("PermissionDescriptions");
  return (key: string, fallback = "") => (isKnownPermission(key) ? t(descriptionKeys[key]) : fallback);
}
