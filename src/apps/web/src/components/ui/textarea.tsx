import * as React from "react"
import { cn } from "cn"

function Textarea({ className, variant, rows, ...props }: React.ComponentProps<"textarea"> & {
  variant?: "three-lines"
}) {
  return (
    <textarea
      data-slot="textarea"
      rows={variant === "three-lines" ? 3 : rows}
      className={cn(
        "flex field-sizing-content min-h-16 w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-base transition-colors outline-none placeholder:text-muted-foreground focus-visible:border-ring disabled:cursor-not-allowed disabled:bg-input/50 disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm dark:bg-input/30 dark:disabled:bg-input/80 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40",
        variant === "three-lines" && "field-sizing-fixed min-h-0 min-w-0 resize-none overflow-y-auto text-xs/5 md:text-xs/5",
        className
      )}
      {...props}
    />
  )
}

export { Textarea }
