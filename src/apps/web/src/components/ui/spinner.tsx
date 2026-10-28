
import { useTranslations } from "next-intl";
import { cn } from "cn"
import { Loader2Icon } from "lucide-react"

function Spinner({ className, ...props }: React.ComponentProps<"svg">) {
const t = useTranslations("Common");

  return (
    <Loader2Icon data-slot="spinner" role="status" aria-label={t("loading")} className={cn("size-4 animate-spin", className)} {...props} />
  )
}

export { Spinner }
