import { useFormatter, useTranslations } from "next-intl";

import type { ChartConfig } from "@/components/ui/chart";

// 工作台示例数据：只表达安全运营的业务形状，尚未接入真实数据源。
// 真实接口落地时整份替换即可，组件与排版不依赖这里的取值。
export function useWorkbenchData() {
  const t = useTranslations("Dashboard");
  const format = useFormatter();

  // 每 5 天一个周期：接入告警量与其中被判为噪声并抑制的量。
  const leadsChartData = [
    { date: "1-5", newLeads: 1840, disqualified: 1120 },
    { date: "6-10", newLeads: 2160, disqualified: 1340 },
    { date: "11-15", newLeads: 1720, disqualified: 980 },
    { date: "16-20", newLeads: 2480, disqualified: 1510 },
    { date: "21-25", newLeads: 2910, disqualified: 1720 },
    { date: "26-30", newLeads: 2340, disqualified: 1380 },
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

  // 完成研判的告警量（含确认为事件与确认为误报）。
  const proposalsChartData = [
    { date: "1-5", proposalsSent: 412 },
    { date: "6-10", proposalsSent: 528 },
    { date: "11-15", proposalsSent: 366 },
    { date: "16-20", proposalsSent: 604 },
    { date: "21-25", proposalsSent: 578 },
    { date: "26-30", proposalsSent: 496 },
  ];

  const proposalsChartConfig = {
    proposalsSent: {
      label: t("proposalsSent"),
      color: "var(--chart-1)",
    },
  } as ChartConfig;

  // 逐月避免损失（美元）：拦截成功的攻击造成的潜在损失估算。
  const revenueChartData = [
    { month: format.dateTime(new Date("2024-07-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 670000 },
    { month: format.dateTime(new Date("2024-08-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 712000 },
    { month: format.dateTime(new Date("2024-09-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 685000 },
    { month: format.dateTime(new Date("2024-10-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 748000 },
    { month: format.dateTime(new Date("2024-11-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 806000 },
    { month: format.dateTime(new Date("2024-12-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 934000 },
    { month: format.dateTime(new Date("2025-01-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 792000 },
    { month: format.dateTime(new Date("2025-02-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 848000 },
    { month: format.dateTime(new Date("2025-03-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 902000 },
    { month: format.dateTime(new Date("2025-04-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 978000 },
    { month: format.dateTime(new Date("2025-05-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 1044000 },
    { month: format.dateTime(new Date("2025-06-01T00:00:00Z"), { month: "short", year: "numeric" }), revenue: 1128000 },
  ];

  const revenueChartConfig = {
    revenue: {
      label: t("revenue"),
      color: "var(--chart-1)",
    },
  } as ChartConfig;

  // 告警按检测源分布：终端、流量、边界、身份与云、情报命中。
  const leadsBySourceChartData = [
    { source: "website", leads: 4820, fill: "var(--color-website)" },
    { source: "referral", leads: 2160, fill: "var(--color-referral)" },
    { source: "social", leads: 1680, fill: "var(--color-social)" },
    { source: "cold", leads: 940, fill: "var(--color-cold)" },
    { source: "other", leads: 610, fill: "var(--color-other)" },
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

  // 每个检测域的规则覆盖：已启用规则数 vs 目标规则数。
  const projectRevenueChartData = [
    { name: t("mvpDevelopment"), actual: 82, target: 90 },
    { name: t("consultation"), actual: 48, target: 65 },
    { name: t("framerSites"), actual: 34, target: 45 },
    { name: t("devopsSupport"), actual: 77, target: 90 },
    { name: t("llmTraining"), actual: 68, target: 80 },
    { name: t("productLaunch"), actual: 52, target: 70 },
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

  // 告警处置漏斗：接入 → 已确认 → 已升级 → 处置中 → 已闭环。
  const salesPipelineChartData = [
    { stage: t("leads"), value: 10210, fill: "var(--chart-1)" },
    { stage: t("qualified"), value: 4860, fill: "var(--chart-2)" },
    { stage: t("proposalSent"), value: 1290, fill: "var(--chart-3)" },
    { stage: t("negotiation"), value: 412, fill: "var(--chart-4)" },
    { stage: t("won"), value: 268, fill: "var(--chart-5)" },
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

  // 各网络域的避免损失（美元）与环比变化。
  const regionSalesData = [
    {
      region: t("northAmerica"),
      sales: 378000,
      percentage: 31,
      growth: "-3.2%",
      isPositive: false,
    },
    {
      region: t("europe"),
      sales: 401000,
      percentage: 34,
      growth: "+9.4%",
      isPositive: true,
    },
    {
      region: t("asiaPacific"),
      sales: 309500,
      percentage: 26,
      growth: "+12.8%",
      isPositive: true,
    },
    {
      region: t("latinAmerica"),
      sales: 122000,
      percentage: 7,
      growth: "-1.7%",
      isPositive: false,
    },
    {
      region: t("middleEastAfrica"),
      sales: 24500,
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
    {
      id: 4,
      title: t("tuneDetection"),
      desc: t("tuneDetectionDesc"),
      due: t("dueThisWeek"),
      priority: "High" as const,
      priorityColor: "bg-red-100 text-red-700",
      checked: false,
    },
    {
      id: 5,
      title: t("intelRefresh"),
      desc: t("intelRefreshDesc"),
      due: t("dueTomorrow"),
      priority: "Medium" as const,
      priorityColor: "bg-yellow-100 text-yellow-700",
      checked: true,
    },
    {
      id: 6,
      title: t("assetSync"),
      desc: t("assetSyncDesc"),
      due: t("dueToday"),
      priority: "Medium" as const,
      priorityColor: "bg-yellow-100 text-yellow-700",
      checked: false,
    },
  ];

  // 最新告警：编号、告警标题、资产、状态、检测源与最近活动时间。
  const recentLeadsData = [
    {
      id: "ALT-4821",
      name: "财务终端检测到凭据填充攻击",
      company: "EDR · FIN-WS-014",
      status: t("new"),
      statusKey: "new",
      source: t("website"),
      lastActivity: t("activityMinutes", { count: 5 }),
    },
    {
      id: "ALT-4817",
      name: "域控账户非工作时段异常登录",
      company: "IAM · CN-SH-DC01",
      status: t("contacted"),
      statusKey: "contacted",
      source: t("referral"),
      lastActivity: t("activityHours", { count: 2 }),
    },
    {
      id: "ALT-4812",
      name: "数据库服务器横向端口扫描",
      company: "NDR · PROD-SEG-3",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("social"),
      lastActivity: t("activityDays", { count: 3 }),
    },
    {
      id: "ALT-4809",
      name: "对象存储桶疑似公开可读",
      company: "Cloud · s3://spx-logs",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("cold"),
      lastActivity: t("activityMinutes", { count: 14 }),
    },
    {
      id: "ALT-4806",
      name: "员工邮箱命中钓鱼附件投放",
      company: "Mail · o365-exchange",
      status: t("negotiation"),
      statusKey: "negotiation",
      source: t("other"),
      lastActivity: t("activityHours", { count: 5 }),
    },
    {
      id: "ALT-4802",
      name: "生产网段出现疑似 C2 心跳",
      company: "NDR · egress-proxy",
      status: t("won"),
      statusKey: "won",
      source: t("website"),
      lastActivity: t("activityDays", { count: 2 }),
    },
    {
      id: "ALT-4798",
      name: "WAF 拦截批量 SQL 注入探测",
      company: "WAF · edge-gw-2",
      status: t("won"),
      statusKey: "won",
      source: t("referral"),
      lastActivity: t("activityMinutes", { count: 23 }),
    },
    {
      id: "ALT-4795",
      name: "特权 Pod 创建疑似容器逃逸",
      company: "EDR · K8S-NODE-07",
      status: t("negotiation"),
      statusKey: "negotiation",
      source: t("social"),
      lastActivity: t("activityHours", { count: 8 }),
    },
    {
      id: "ALT-4791",
      name: "堡垒机多次失败后登录成功",
      company: "PAM · bastion-01",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("cold"),
      lastActivity: t("activityDays", { count: 1 }),
    },
    {
      id: "ALT-4788",
      name: "出口流量命中已知矿池域名",
      company: "NDR · egress-proxy",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("other"),
      lastActivity: t("activityMinutes", { count: 32 }),
    },
    {
      id: "ALT-4784",
      name: "非授权设备接入生产 VLAN",
      company: "Net · VLAN-210",
      status: t("contacted"),
      statusKey: "contacted",
      source: t("website"),
      lastActivity: t("activityHours", { count: 2 }),
    },
    {
      id: "ALT-4780",
      name: "管理员账号短时间多点登录",
      company: "IAM · CN-SH-DC01",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("referral"),
      lastActivity: t("activityDays", { count: 4 }),
    },
    {
      id: "ALT-4776",
      name: "共享盘出现勒索扩展名批量改写",
      company: "Share · fileserver-3",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("social"),
      lastActivity: t("activityMinutes", { count: 41 }),
    },
    {
      id: "ALT-4772",
      name: "计划任务持久化（schtasks 创建）",
      company: "EDR · HR-WS-022",
      status: t("negotiation"),
      statusKey: "negotiation",
      source: t("cold"),
      lastActivity: t("activityHours", { count: 5 }),
    },
    {
      id: "ALT-4768",
      name: "疑似 Kerberoasting 票据请求",
      company: "IAM · AD-LEGACY",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("other"),
      lastActivity: t("activityDays", { count: 3 }),
    },
    {
      id: "ALT-4765",
      name: "DNS 隧道特征（长随机子域）",
      company: "DNS · resolver-1",
      status: t("contacted"),
      statusKey: "contacted",
      source: t("website"),
      lastActivity: t("activityMinutes", { count: 50 }),
    },
    {
      id: "ALT-4761",
      name: "RDP 弱口令并发爆破",
      company: "EDR · RDP-GW",
      status: t("won"),
      statusKey: "won",
      source: t("referral"),
      lastActivity: t("activityHours", { count: 8 }),
    },
    {
      id: "ALT-4757",
      name: "内部 API 出现越权访问序列",
      company: "API · gw-internal",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("social"),
      lastActivity: t("activityDays", { count: 2 }),
    },
    {
      id: "ALT-4753",
      name: "安全组放通 0.0.0.0/0 高危端口",
      company: "Cloud · sg-prod-4",
      status: t("new"),
      statusKey: "new",
      source: t("cold"),
      lastActivity: t("activityMinutes", { count: 59 }),
    },
    {
      id: "ALT-4749",
      name: "EDR 代理被尝试卸载",
      company: "EDR · DEV-BOX-9",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("other"),
      lastActivity: t("activityHours", { count: 2 }),
    },
    {
      id: "ALT-4746",
      name: "敏感文件批量下载超阈值",
      company: "DLP · share-2",
      status: t("negotiation"),
      statusKey: "negotiation",
      source: t("website"),
      lastActivity: t("activityDays", { count: 1 }),
    },
    {
      id: "ALT-4742",
      name: "VPN 账号异常地理位置登录",
      company: "VPN · gw-remote",
      status: t("won"),
      statusKey: "won",
      source: t("referral"),
      lastActivity: t("activityMinutes", { count: 68 }),
    },
    {
      id: "ALT-4738",
      name: "证书过期导致合规项失败",
      company: "PKI · ca-internal",
      status: t("contacted"),
      statusKey: "contacted",
      source: t("social"),
      lastActivity: t("activityHours", { count: 5 }),
    },
    {
      id: "ALT-4734",
      name: "日志采集断流 30 分钟",
      company: "Ingest · syslog-5",
      status: t("won"),
      statusKey: "won",
      source: t("cold"),
      lastActivity: t("activityDays", { count: 4 }),
    },
    {
      id: "ALT-4730",
      name: "sudo 越权调用疑似提权",
      company: "EDR · FIN-WS-021",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("other"),
      lastActivity: t("activityMinutes", { count: 77 }),
    },
    {
      id: "ALT-4726",
      name: "外观相似域名仿冒邮件域",
      company: "Mail · gateway-1",
      status: t("new"),
      statusKey: "new",
      source: t("website"),
      lastActivity: t("activityHours", { count: 8 }),
    },
    {
      id: "ALT-4722",
      name: "依赖命中已知恶意包版本",
      company: "CI · pipeline-build",
      status: t("qualified"),
      statusKey: "qualified",
      source: t("referral"),
      lastActivity: t("activityDays", { count: 3 }),
    },
    {
      id: "ALT-4718",
      name: "长期未登录的冗余账户",
      company: "IAM · AD-LEGACY",
      status: t("won"),
      statusKey: "won",
      source: t("social"),
      lastActivity: t("activityMinutes", { count: 86 }),
    },
    {
      id: "ALT-4714",
      name: "Modbus 异常写操作（产线）",
      company: "OT · plc-line2",
      status: t("negotiation"),
      statusKey: "negotiation",
      source: t("cold"),
      lastActivity: t("activityHours", { count: 2 }),
    },
    {
      id: "ALT-4710",
      name: "云 IAM 角色信任策略被放宽",
      company: "Cloud · role/etl",
      status: t("proposalSent"),
      statusKey: "proposalSent",
      source: t("other"),
      lastActivity: t("activityDays", { count: 2 }),
    },
    {
      id: "ALT-4706",
      name: "安全策略变更未走审批",
      company: "IAM · policy-store",
      status: t("new"),
      statusKey: "new",
      source: t("website"),
      lastActivity: t("activityMinutes", { count: 95 }),
    },
    {
      id: "ALT-4702",
      name: "办公网批量文件共享外发",
      company: "DLP · share-1",
      status: t("contacted"),
      statusKey: "contacted",
      source: t("referral"),
      lastActivity: t("activityHours", { count: 5 }),
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
