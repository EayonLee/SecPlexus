import type { EChartsOption, SeriesOption } from 'echarts';
export type ChartRow = Record<string, string | number>;

// Chart colors follow the original Pro chart specifications, independently of
// the UI primary color. Each chart can supply its own semantic palette.
export const palette = ['#249EFF', '#846BCE', '#21CCFF', '#86DF6C', '#0E42D2'];
export const linePalette = ['#21CCFF', '#313CA9', '#249EFF'];
export const publishingPalette = ['#81E2FF', '#00B2FF', '#246EFF'];
export const formatThousands = (value: string | number) =>
  `${Number(value) / 1000}k`;

export function verticalGradient(top: string, bottom: string) {
  return {
    type: 'linear' as const,
    x: 0,
    y: 0,
    x2: 0,
    y2: 1,
    colorStops: [
      { offset: 0, color: top },
      { offset: 1, color: bottom },
    ],
  };
}

export function cartesian(
  data: ChartRow[] = [],
  x = 'date',
  y = 'count',
  group?: string,
  type: 'line' | 'bar' = 'line',
  area = false,
  compact = false,
  horizontal = false,
  stack = false,
) {
  const labels = [...new Set(data.map((row) => String(row[x])))];
  const groups = group
    ? [...new Set(data.map((row) => String(row[group])))]
    : [''];
  const series: SeriesOption[] = groups.map((name) => ({
    name,
    type,
    smooth: type === 'line',
    symbol: 'none',
    lineStyle: { width: 2 },
    barMaxWidth: 16,
    stack: stack ? 'total' : undefined,
    areaStyle: area ? { opacity: 0.15 } : undefined,
    data: labels.map((label) => {
      const row = data.find(
        (row) =>
          String(row[x]) === label && (!group || String(row[group]) === name),
      );
      return row ? Number(row[y]) : null;
    }),
  }));
  const category = {
    type: 'category' as const,
    data: labels,
    show: !compact,
    boundaryGap: type === 'bar',
    axisTick: { show: false },
    splitLine: { show: false },
    axisLabel: { fontSize: 10 },
  };
  const value = {
    type: 'value' as const,
    show: !compact,
    axisLine: { show: false },
    axisLabel: { fontSize: 10 },
    splitLine: { lineStyle: { type: 'solid' as const } },
  };
  return {
    color: palette,
    grid: compact
      ? { top: 10, bottom: 0, left: 10, right: 10 }
      : { top: 15, bottom: group ? 55 : 35, left: 45, right: 20 },
    tooltip: { show: !compact, trigger: 'axis' },
    legend: {
      show: !compact && !!group,
      bottom: 12,
      icon: 'circle',
      itemWidth: 6,
      itemHeight: 6,
      textStyle: { fontSize: 10 },
    },
    xAxis: horizontal ? value : category,
    yAxis: horizontal ? category : value,
    series,
  } satisfies EChartsOption;
}

export function donut(
  data: ChartRow[] = [],
  name = 'type',
  value = 'count',
  compact = false,
): EChartsOption {
  return {
    color: compact ? ['#8D4EDA', '#00B2FF', '#165DFF'] : palette,
    tooltip: { trigger: 'item' },
    legend: {
      ...(compact
        ? { orient: 'vertical', right: 0, top: 'middle' }
        : { bottom: 0 }),
      icon: 'circle',
      itemWidth: 6,
      itemHeight: 6,
      textStyle: { fontSize: compact ? 10 : 12 },
    },
    series: [
      {
        type: 'pie',
        selectedMode: 'single',
        radius: compact ? ['56%', '80%'] : ['55%', '75%'],
        center: ['50%', compact ? '50%' : '44%'],
        left: 0,
        right: compact ? 56 : 0,
        label: {
          show: !compact,
          formatter: '{d}%',
          alignTo: 'edge',
          edgeDistance: 12,
          width: 36,
          overflow: 'break',
        },
        data: data.map((row) => ({
          name: String(row[name]),
          value: Number(row[value]),
        })),
      },
    ],
  };
}
