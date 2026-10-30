import type { ComponentProps, ReactNode } from "react";

import { Badge } from "@/components/ui/badge";

const statusVariants: Record<string, ComponentProps<typeof Badge>["variant"]> = {
  active: "success",
  success: "success",
  won: "success",
  qualified: "success",
  disabled: "destructive",
  suspended: "destructive",
  failure: "destructive",
  denied: "warning",
  pending: "warning",
  negotiation: "warning",
  current: "info",
  system: "info",
  contacted: "info",
  proposalSent: "info",
  new: "info",
};

/** StatusBadge 统一业务状态的颜色语义，未知分类保持中性并保留原始文字。 */
export function StatusBadge({ status = "", children }: { status?: string; children: ReactNode }) {
  return <Badge variant={statusVariants[status] ?? "secondary"}>{children}</Badge>;
}
