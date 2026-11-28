import type { ReactNode } from "react";

import { Card } from "@/components/ui/card";

/** 标题、筛选、记录与分页共用模板的 Card 表面。 */
export function ListSurface({ children }: { children: ReactNode }) {
  return <Card className="@container/list min-w-0 gap-0 py-0">{children}</Card>;
}

export function Toolbar({ children, actions }: { children?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="flex flex-col gap-3 p-4 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
      {children && <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">{children}</div>}
      {actions && <div className="ml-auto flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}
