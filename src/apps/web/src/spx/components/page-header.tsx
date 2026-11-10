import type { ReactNode } from "react";

/**
 * PageHeader 是设置区各页面共用的标题行：标题在左，主操作在右。
 * 标题字号沿用 Web 模板基线（dashboard/roles），页面不再各自定义标题样式。
 */
export function PageHeader({ title, actions }: { title: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-col items-start gap-4 sm:flex-row sm:justify-between">
      <h1 className="text-3xl tracking-tight">{title}</h1>
      {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
    </div>
  );
}
