import type { ReactNode } from "react";

export function PageTitle({ children }: { children: ReactNode }) {
  return <h1 className="text-2xl font-semibold tracking-tight break-words">{children}</h1>;
}

/** 工作台、设置和业务子页共用页首。标题旁只放当前页面的操作。 */
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <header className="flex min-w-0 flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 flex-col gap-1">
        <PageTitle>{title}</PageTitle>
        {description && <p className="text-muted-foreground text-sm">{description}</p>}
      </div>
      {actions && <div className="flex max-w-full flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}
