"use client";
import { subMonths } from "date-fns";
import { BadgeDollarSign, Wallet } from "lucide-react";
import { useFormatter, useNow, useTranslations } from "next-intl";
import { Area, AreaChart, Bar, BarChart, Line, LineChart, XAxis } from "recharts";

import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart";

import { useCRMData } from "./crm.config";

export function OverviewCards() {
  const t = useTranslations("Dashboard");
  const format = useFormatter();
  const now = useNow();
  const lastMonth = format.dateTime(subMonths(now, 1), { month: "long" });

  const {
    leadsChartConfig,
    leadsChartData,
    proposalsChartConfig,
    proposalsChartData,
    revenueChartConfig,
    revenueChartData,
  } = useCRMData();
  return (
    <div className="grid grid-cols-1 gap-4 *:data-[slot=card]:shadow-xs sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("newLeads")}</CardTitle>
          <CardDescription>{t("lastMonth")}</CardDescription>
        </CardHeader>
        <CardContent className="size-full">
          <ChartContainer className="size-full min-h-24" config={leadsChartConfig}>
            <BarChart accessibilityLayer data={leadsChartData} barSize={8}>
              <XAxis dataKey="date" tickLine={false} tickMargin={10} axisLine={false} hide />
              <ChartTooltip
                content={
                  <ChartTooltipContent
                    labelFormatter={(label) => t("periodLabel", { month: lastMonth, range: String(label) })}
                  />
                }
              />
              <Bar
                background={{ fill: "var(--color-background)", radius: 4, opacity: 0.07 }}
                dataKey="newLeads"
                stackId="a"
                fill="var(--color-newLeads)"
                radius={[0, 0, 0, 0]}
              />
              <Bar dataKey="disqualified" stackId="a" fill="var(--color-disqualified)" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ChartContainer>
        </CardContent>
        <CardFooter className="flex items-center justify-between">
          <span className="text-xl font-semibold tabular-nums">{format.number(635)}</span>
          <span className="text-sm font-medium text-green-500">+54.6%</span>
        </CardFooter>
      </Card>

      <Card className="overflow-hidden pb-0">
        <CardHeader>
          <CardTitle>{t("proposalsSent")}</CardTitle>
          <CardDescription>{t("lastMonth")}</CardDescription>
        </CardHeader>
        <CardContent className="flex-1 p-0">
          <ChartContainer className="size-full min-h-24" config={proposalsChartConfig}>
            <AreaChart
              data={proposalsChartData}
              margin={{
                left: 0,
                right: 0,
                top: 5,
              }}
            >
              <XAxis dataKey="date" tickLine={false} tickMargin={10} axisLine={false} hide />
              <ChartTooltip
                content={
                  <ChartTooltipContent
                    labelFormatter={(label) => t("periodLabel", { month: lastMonth, range: String(label) })}
                    hideIndicator
                  />
                }
              />
              <Area
                dataKey="proposalsSent"
                fill="var(--color-proposalsSent)"
                fillOpacity={0.05}
                stroke="var(--color-proposalsSent)"
                strokeWidth={2}
                type="monotone"
              />
            </AreaChart>
          </ChartContainer>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="w-fit rounded-lg bg-green-500/10 p-2">
            <Wallet className="size-5 text-green-500" />
          </div>
        </CardHeader>
        <CardContent className="flex size-full flex-col justify-between">
          <div className="space-y-1.5">
            <CardTitle>{t("revenue")}</CardTitle>
            <CardDescription>{t("lastSixMonths")}</CardDescription>
          </div>
          <p className="text-2xl font-medium tabular-nums">
            {format.number(56050, { style: "currency", currency: "USD", maximumFractionDigits: 0 })}
          </p>
          <div className="w-fit rounded-md bg-green-500/10 px-2 py-1 text-xs font-medium text-green-500">+22.2%</div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="bg-destructive/10 w-fit rounded-lg p-2">
            <BadgeDollarSign className="text-destructive size-5" />
          </div>
        </CardHeader>
        <CardContent className="flex size-full flex-col justify-between">
          <div className="space-y-1.5">
            <CardTitle>{t("projectsWon")}</CardTitle>
            <CardDescription>{t("lastSixMonths")}</CardDescription>
          </div>
          <p className="text-2xl font-medium tabular-nums">{format.number(136)}</p>
          <div className="bg-destructive/10 text-destructive w-fit rounded-md px-2 py-1 text-xs font-medium">-2.5%</div>
        </CardContent>
      </Card>

      <Card className="col-span-1 xl:col-span-2">
        <CardHeader>
          <CardTitle>{t("revenueGrowth")}</CardTitle>
          <CardDescription>{t("yearToDate")}</CardDescription>
        </CardHeader>
        <CardContent>
          <ChartContainer config={revenueChartConfig} className="h-24 w-full">
            <LineChart
              data={revenueChartData}
              margin={{
                top: 5,
                right: 10,
                left: 10,
                bottom: 0,
              }}
            >
              <XAxis dataKey="month" tickLine={false} tickMargin={10} axisLine={false} hide />
              <ChartTooltip content={<ChartTooltipContent />} />
              <Line
                type="monotone"
                strokeWidth={2}
                dataKey="revenue"
                stroke="var(--color-revenue)"
                activeDot={{
                  r: 6,
                }}
              />
            </LineChart>
          </ChartContainer>
        </CardContent>
        <CardFooter>
          <p className="text-muted-foreground text-sm">{t("annualGrowth")}</p>
        </CardFooter>
      </Card>
    </div>
  );
}
