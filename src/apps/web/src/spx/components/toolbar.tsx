import type { ReactNode } from "react";

/**
 * Toolbar 是列表页工具条的唯一结构：查询条件在左，操作按钮成组靠右。
 *
 * 各页不再自行拼 flex 容器，避免按钮位置与尺寸在不同页面漂移；
 * 传入的按钮统一使用 size="sm"，与 h-8 的输入框同高。
 */
export function Toolbar({ children, actions }: { children?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {children}
      {actions ? <div className="ml-auto flex items-center gap-2">{actions}</div> : null}
    </div>
  );
}
