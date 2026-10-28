import type { components } from "@/auth/api.gen";

export type Identity = components["schemas"]["APIMe"];
export type AuthStatus = "unknown" | "anonymous" | "authenticated" | "unavailable";
export interface AuthState extends Omit<Identity, "principal"> {
  principal?: Identity["principal"];
  status: AuthStatus;
}
export const initialAuth: AuthState = {
  status: "unknown",
  organizations: [],
  capabilities: [],
  platform_capabilities: [],
};
