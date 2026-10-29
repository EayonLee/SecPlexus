import type { ApiMe } from "@/spx/api/iam/types.gen";

export type AuthStatus = "unknown" | "anonymous" | "authenticated" | "unavailable";
export interface AuthState extends Omit<ApiMe, "principal"> {
  principal?: ApiMe["principal"];
  status: AuthStatus;
}
export const initialAuth: AuthState = {
  status: "unknown",
  organizations: [],
  capabilities: [],
  platform_capabilities: [],
};
