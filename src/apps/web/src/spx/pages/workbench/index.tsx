import { useTranslations } from "next-intl";

import { useCRMData } from "./_components/crm.config";
import { InsightCards } from "./_components/insight-cards";
import { OperationalCards } from "./_components/operational-cards";
import { OverviewCards } from "./_components/overview-cards";
import { RecentLeadsTable } from "./_components/recent-leads-table/table";

export default function Page() {
  const t = useTranslations("Dashboard");

  const { recentLeadsData } = useCRMData();
  return (
    <div className="flex flex-col gap-4 md:gap-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
        <p className="text-muted-foreground text-sm">{t("sampleNotice")}</p>
      </header>
      <OverviewCards />
      <InsightCards />
      <OperationalCards />
      <RecentLeadsTable data={recentLeadsData} />
    </div>
  );
}
