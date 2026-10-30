import { useFormatter, useTranslations } from "next-intl";

import type { ChartConfig } from "@/components/ui/chart";

export function useCRMData() {
  const t = useTranslations("Dashboard");
  const format = useFormatter();

  const leadsChartData = [
    { date: "1-5", newLeads: 120, disqualified: 40 },
    { date: "6-10", newLeads: 95, disqualified: 30 },
    { date: "11-15", newLeads: 60, disqualified: 22 },
    { date: "16-20", newLeads: 100, disqualified: 35 },
    { date: "21-25", newLeads: 150, disqualified: 70 },
    { date: "26-30", newLeads: 110, disqualified: 60 },
  ];

  const leadsChartConfig = {
    newLeads: {
      label: t("newLeads"),
      color: "var(--chart-1)",
    },
    disqualified: {
      label: t("disqualified"),
      color: "var(--chart-3)",
    },
    background: {
      color: "var(--primary)",
    },
  } as ChartConfig;

  const proposalsChartData = [
    { date: "1-5", proposalsSent: 9 },
    { date: "6-10", proposalsSent: 16 },
    { date: "11-15", proposalsSent: 6 },
    { date: "16-20", proposalsSent: 18 },
    { date: "21-25", proposalsSent: 11 },
    { date: "26-30", proposalsSent: 14 },
  ];

  const proposalsChartConfig = {
    proposalsSent: {
      label: t("proposalsSent"),
      color: "var(--chart-1)",
    },
  } as ChartConfig;

  const revenueChartData = [
    { month: format.dateTime(new Date("2024-07-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 6700 },
    { month: format.dateTime(new Date("2024-08-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 7100 },
    { month: format.dateTime(new Date("2024-09-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 6850 },
    { month: format.dateTime(new Date("2024-10-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 7500 },
    { month: format.dateTime(new Date("2024-11-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 8000 },
    { month: format.dateTime(new Date("2024-12-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 8300 },
    { month: format.dateTime(new Date("2025-01-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 7900 },
    { month: format.dateTime(new Date("2025-02-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 8400 },
    { month: format.dateTime(new Date("2025-03-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 8950 },
    { month: format.dateTime(new Date("2025-04-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 9700 },
    { month: format.dateTime(new Date("2025-05-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 11200 },
    { month: format.dateTime(new Date("2025-06-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 9500 },
  ];

  const revenueChartConfig = {
    revenue: {
      label: t("revenue"),
      color: "var(--chart-1)",
    },
  } as ChartConfig;

  const leadsBySourceChartData = [
    { source: "website", leads: 170, fill: "var(--color-website)" },
    { source: "referral", leads: 105, fill: "var(--color-referral)" },
    { source: "social", leads: 90, fill: "var(--color-social)" },
    { source: "cold", leads: 62, fill: "var(--color-cold)" },
    { source: "other", leads: 48, fill: "var(--color-other)" },
  ];

  const leadsBySourceChartConfig = {
    leads: {
      label: t("leads"),
    },
    website: {
      label: t("website"),
      color: "var(--chart-1)",
    },
    referral: {
      label: t("referral"),
      color: "var(--chart-2)",
    },
    social: {
      label: t("social"),
      color: "var(--chart-3)",
    },
    cold: {
      label: t("cold"),
      color: "var(--chart-4)",
    },
    other: {
      label: t("other"),
      color: "var(--chart-5)",
    },
  } as ChartConfig;

  const projectRevenueChartData = [
    { name: t("mvpDevelopment"), actual: 82000, target: 90000 },
    { name: t("consultation"), actual: 48000, target: 65000 },
    { name: t("framerSites"), actual: 34000, target: 45000 },
    { name: t("devopsSupport"), actual: 77000, target: 90000 },
    { name: t("llmTraining"), actual: 68000, target: 80000 },
    { name: t("productLaunch"), actual: 52000, target: 70000 },
  ].map((row) => ({
    ...row,
    remaining: Math.max(0, row.target - row.actual),
  }));

  const projectRevenueChartConfig = {
    actual: {
      label: t("actual"),
      color: "var(--chart-1)",
    },
    remaining: {
      label: t("remaining"),
      color: "var(--chart-2)",
    },
    label: {
      color: "var(--primary-foreground)",
    },
  } as ChartConfig;

  const salesPipelineChartData = [
    { stage: t("leads"), value: 680, fill: "var(--chart-1)" },
    { stage: t("qualified"), value: 480, fill: "var(--chart-2)" },
    { stage: t("proposalSent"), value: 210, fill: "var(--chart-3)" },
    { stage: t("negotiation"), value: 120, fill: "var(--chart-4)" },
    { stage: t("won"), value: 45, fill: "var(--chart-5)" },
  ];

  const salesPipelineChartConfig = {
    value: {
      label: t("leads"),
      color: "var(--chart-1)",
    },
    stage: {
      label: t("stage"),
    },
  } as ChartConfig;

  const regionSalesData = [
    {
      region: t("northAmerica"),
      sales: 37800,
      percentage: 31,
      growth: "-3.2%",
      isPositive: false,
    },
    {
      region: t("europe"),
      sales: 40100,
      percentage: 34,
      growth: "+9.4%",
      isPositive: true,
    },
    {
      region: t("asiaPacific"),
      sales: 30950,
      percentage: 26,
      growth: "+12.8%",
      isPositive: true,
    },
    {
      region: t("latinAmerica"),
      sales: 12200,
      percentage: 7,
      growth: "-1.7%",
      isPositive: false,
    },
    {
      region: t("middleEastAfrica"),
      sales: 2450,
      percentage: 2,
      growth: "+6.0%",
      isPositive: true,
    },
  ];

  const actionItems = [
    {
      id: 1,
      title: t("kickoff"),
      desc: t("onboardingDocs"),
      due: t("dueToday"),
      priority: "High" as const,
      priorityColor: "bg-red-100 text-red-700",
      checked: false,
    },
    {
      id: 2,
      title: t("demoCall"),
      desc: t("bookCall"),
      due: t("dueTomorrow"),
      priority: "Medium" as const,
      priorityColor: "bg-yellow-100 text-yellow-700",
      checked: true,
    },
    {
      id: 3,
      title: t("caseStudy"),
      desc: t("addLlmProject"),
      due: t("dueThisWeek"),
      priority: "Low" as const,
      priorityColor: "bg-green-100 text-green-700",
      checked: false,
    },
  ];

  const recentLeadsData = [
    {
      id: "L-1012",
      name: "Guillermo Rauch",
      company: "Vercel",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("website"),
      lastActivity: t("activityMinutes", { count: 30 }),
    },
    {
      id: "L-1018",
      name: "Nizzy",
      company: "Mail0",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("website"),
      lastActivity: t("activityMinutes", { count: 35 }),
    },
    {
      id: "L-1005",
      name: "Sahaj",
      company: "Tweakcn",
      status: t("negotiation"),
      statusKey: "negotiation",
      source: t("website"),
      lastActivity: t("activityHours", { count: 1 }),
    },
    {
      id: "L-1001",
      name: "Shadcn",
      company: "Shadcn/ui",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("website"),
      lastActivity: t("activityHours", { count: 2 }),
    },
    {
      id: "L-1003",
      name: "Sam Altman",
      company: "OpenAI",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("social"),
      lastActivity: t("activityHours", { count: 4 }),
    },
    {
      id: "L-1008",
      name: "Michael Andreuzza",
      company: "Lexington Themes",
      status: t("contacted"),
      statusKey: "contacted",
      source: t("social"),
      lastActivity: t("activityHours", { count: 5 }),
    },
    {
      id: "L-1016",
      name: "Skyleen",
      company: "Animate UI",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("referral"),
      lastActivity: t("activityHours", { count: 7 }),
    },
    {
      id: "L-1007",
      name: "SecPlexus Demo",
      company: "Weblabs Studio",
      status: t("won"),
      statusKey: "won",
      source: t("website"),
      lastActivity: t("activityHours", { count: 6 }),
    },
    {
      id: "L-1011",
      name: "Sebastian Rindom",
      company: "Medusa",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("referral"),
      lastActivity: t("activityHours", { count: 10 }),
    },
    {
      id: "L-1014",
      name: "Fred K. Schott",
      company: "Astro",
      status: t("contacted"),
      statusKey: "contacted",
      source: t("social"),
      lastActivity: t("activityHours", { count: 12 }),
    },
    {
      id: "L-1010",
      name: "Peer Richelsen",
      company: "Cal.com",
      status: t("new"),
      statusKey: "new",
      source: t("other"),
      lastActivity: t("activityHours", { count: 8 }),
    },
    {
      id: "L-1002",
      name: "Ammar Khnz",
      company: "BE",
      status: t("contacted"),
      statusKey: "contacted",
      source: t("referral"),
      lastActivity: t("activityDays", { count: 1 }),
    },
    {
      id: "L-1015",
      name: "Toby",
      company: "Shadcn UI Kit ",
      status: t("negotiation"),
      statusKey: "negotiation",
      source: t("other"),
      lastActivity: t("activityDays", { count: 2 }),
    },
    {
      id: "L-1006",
      name: "David Haz",
      company: "React Bits",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("referral"),
      lastActivity: t("activityDays", { count: 2 }),
    },
    {
      id: "L-1004",
      name: "Erşad",
      company: "Align UI",
      status: t("new"),
      statusKey: "new",
      source: t("cold"),
      lastActivity: t("activityDays", { count: 3 }),
    },
  ];

  return {
    leadsChartData,
    leadsChartConfig,
    proposalsChartData,
    proposalsChartConfig,
    revenueChartData,
    revenueChartConfig,
    leadsBySourceChartData,
    leadsBySourceChartConfig,
    projectRevenueChartData,
    projectRevenueChartConfig,
    salesPipelineChartData,
    salesPipelineChartConfig,
    regionSalesData,
    actionItems,
    recentLeadsData,
  };
}
