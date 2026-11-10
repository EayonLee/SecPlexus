/**
 * SheetPanel 是全站右侧抽屉的唯一实现。
 *
 * 宽度、头部/正文/底部结构与滚动边界只在这里定义：抽屉宽度曾因各页面自行覆盖
 * `sm:max-w-*` 被组件的 `data-[side=right]:sm:max-w-sm` 压回 24rem，所以宽度也必须在
 * 同一处用同名变体声明，页面不再各自指定。
 */
import type { ReactNode } from "react";

import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";

export function SheetPanel({
  title,
  description,
  footer,
  children,
  onClose,
}: {
  title: string;
  description?: string;
  footer?: ReactNode;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent
        side="right"
        className="flex flex-col gap-0 data-[side=right]:w-full data-[side=right]:max-w-none data-[side=right]:sm:w-[min(72rem,88vw)]"
      >
        <SheetHeader className="border-b px-6 py-4">
          <SheetTitle>{title}</SheetTitle>
          {description && <SheetDescription>{description}</SheetDescription>}
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">{children}</div>
        {footer && <SheetFooter className="flex flex-row justify-end gap-2 border-t px-6 py-4">{footer}</SheetFooter>}
      </SheetContent>
    </Sheet>
  );
}
