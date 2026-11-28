import { type ReactNode, useRef } from "react";

import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";

export function SheetPanel({
  title,
  description,
  footer,
  children,
  onClose,
  layout,
  returnFocus,
}: {
  title: string;
  description?: string;
  footer?: ReactNode;
  children: ReactNode;
  onClose: () => void;
  layout?: "management";
  returnFocus?: HTMLElement | null;
}) {
  const opener = useRef<HTMLElement | null>(null);
  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent
        side="right"
        data-layout={layout}
        {...(!description && { "aria-describedby": undefined })}
        onOpenAutoFocus={() => {
          // 业务抽屉按需挂载，触发按钮位于 Radix 根节点之外。
          if (document.activeElement instanceof HTMLElement) opener.current = document.activeElement;
        }}
        onCloseAutoFocus={(event) => {
          const target = returnFocus ?? opener.current;
          if (target?.isConnected) {
            event.preventDefault();
            requestAnimationFrame(() => {
              if (target.isConnected) target.focus({ preventScroll: true });
            });
          }
        }}
        className="sheet-panel flex flex-col gap-0"
      >
        <SheetHeader className="border-b px-6 py-4">
          <SheetTitle>{title}</SheetTitle>
          {description && <SheetDescription>{description}</SheetDescription>}
        </SheetHeader>
        <div className="sheet-body min-h-0 flex-1 overflow-x-hidden overflow-y-auto">{children}</div>
        {footer && (
          <SheetFooter className="flex flex-row flex-wrap justify-end gap-2 border-t px-6 py-4">{footer}</SheetFooter>
        )}
      </SheetContent>
    </Sheet>
  );
}
