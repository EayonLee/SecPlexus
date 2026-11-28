import { useTranslations } from "next-intl";

import { PageHeader } from "../../components/page-header";
import { AlertsTable } from "./_components/alerts-table/table";
import { InsightCards } from "./_components/insight-cards";
import { OperationalCards } from "./_components/operational-cards";
import { OverviewCards } from "./_components/overview-cards";
import { useWorkbenchData } from "./_components/workbench.config";

export default function Page() {
  const t = useTranslations("Dashboard");

  const { recentLeadsData } = useWorkbenchData();
  return (
    <div className="@container/main flex flex-col gap-4 md:gap-6">
      <PageHeader title={t("title")} />
      <OverviewCards />
      <InsightCards />
      <OperationalCards />
      <AlertsTable data={recentLeadsData} />
    </div>
  );
}
