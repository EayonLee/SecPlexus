import { useContext, useEffect, useRef } from 'react';
import { init, use, type ECharts } from 'echarts/core';
import type {
  EChartsOption,
  XAxisComponentOption,
  YAxisComponentOption,
} from 'echarts';
import { LineChart, BarChart, PieChart, RadarChart } from 'echarts/charts';
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  RadarComponent,
  TitleComponent,
} from 'echarts/components';
import { SVGRenderer } from 'echarts/renderers';
import { GlobalContext } from '@/context';
import { formatChartTooltip } from './customer-tooltip';

use([
  LineChart,
  BarChart,
  PieChart,
  RadarChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  RadarComponent,
  TitleComponent,
  SVGRenderer,
]);

function themeAxes<T extends XAxisComponentOption | YAxisComponentOption>(
  axes: T | T[] | undefined,
  labelColor: string,
  borderColor: string,
): T | T[] | undefined {
  if (!axes) return axes;
  const decorate = (axis: T): T => ({
    ...axis,
    axisLabel: { ...axis.axisLabel, color: labelColor },
    axisLine: {
      ...axis.axisLine,
      lineStyle: { ...axis.axisLine?.lineStyle, color: borderColor },
    },
    splitLine: {
      ...axis.splitLine,
      lineStyle: { ...axis.splitLine?.lineStyle, color: borderColor },
    },
  });
  return Array.isArray(axes) ? axes.map(decorate) : decorate(axes);
}

export default function ChartView({
  option,
  height = 300,
  label = '数据图表',
}: {
  option: EChartsOption;
  height?: number;
  label?: string;
}) {
  const container = useRef<HTMLDivElement>(null);
  const instance = useRef<ECharts>(null);
  const { theme } = useContext(GlobalContext);
  useEffect(() => {
    const chart = init(
      container.current!,
      theme === 'dark' ? 'dark' : undefined,
      { renderer: 'svg' },
    );
    instance.current = chart;
    const observer = new ResizeObserver(() => chart.resize());
    observer.observe(container.current!);
    return () => {
      observer.disconnect();
      chart.dispose();
      instance.current = null;
    };
  }, [theme]);
  useEffect(() => {
    // The parent updates arco-theme in an effect. Read CSS colors on the next
    // frame so chart axes use the new mode rather than the previous one.
    const frame = requestAnimationFrame(() => {
      const styles = getComputedStyle(document.body);
      const labelColor = styles.getPropertyValue('--color-text-3').trim();
      const textColor = styles.getPropertyValue('--color-text-1').trim();
      const borderColor = styles.getPropertyValue('--color-border-2').trim();
      const legends = option.legend
        ? Array.isArray(option.legend)
          ? option.legend
          : [option.legend]
        : undefined;
      const titles = option.title
        ? Array.isArray(option.title)
          ? option.title
          : [option.title]
        : undefined;
      const radars = option.radar
        ? Array.isArray(option.radar)
          ? option.radar
          : [option.radar]
        : undefined;
      instance.current?.setOption(
        {
          ...option,
          backgroundColor: 'transparent',
          tooltip: {
            backgroundColor:
              theme === 'dark' ? 'rgba(42,42,43,0.8)' : 'rgba(255,255,255,0.8)',
            borderWidth: 0,
            padding: 8,
            extraCssText:
              'backdrop-filter:blur(10px);-webkit-backdrop-filter:blur(10px);border-radius:4px;box-shadow:0 2px 10px rgba(0,0,0,0.15);',
            formatter: formatChartTooltip,
            ...option.tooltip,
          },
          textStyle: {
            fontFamily: styles.fontFamily,
            color: textColor,
            ...option.textStyle,
          },
          title: titles?.map((title) => ({
            ...title,
            textStyle: { color: textColor, ...title.textStyle },
          })),
          radar: radars?.map((radar) => ({
            ...radar,
            axisName: { ...radar.axisName, color: labelColor },
            axisLine: {
              ...radar.axisLine,
              lineStyle: { ...radar.axisLine?.lineStyle, color: borderColor },
            },
            splitLine: {
              ...radar.splitLine,
              lineStyle: { ...radar.splitLine?.lineStyle, color: borderColor },
            },
          })),
          xAxis: themeAxes(option.xAxis, labelColor, borderColor),
          yAxis: themeAxes(option.yAxis, labelColor, borderColor),
          legend: legends?.map((legend) => ({
            ...legend,
            textStyle: { color: labelColor, ...legend.textStyle },
          })),
          animation: !window.matchMedia('(prefers-reduced-motion: reduce)')
            .matches,
        },
        { notMerge: true },
      );
    });
    return () => cancelAnimationFrame(frame);
  }, [option, theme]);
  return (
    <div
      ref={container}
      role="img"
      aria-label={label}
      style={{ width: '100%', height }}
    />
  );
}
