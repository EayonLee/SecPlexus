import type { LineSeriesOption } from 'echarts';
import { Spin } from '@arco-design/web-react';
import ChartView from './chart-view';
import {
  cartesian,
  verticalGradient,
  formatThousands,
  type ChartRow,
} from './options';

export default function OverviewAreaLine({
  data,
  loading,
  name = '总内容量',
  color = '#4080FF',
}: {
  data: ChartRow[];
  loading: boolean;
  name?: string;
  color?: string;
}) {
  const option = cartesian(data, 'date', 'count', undefined, 'line', true);
  const series = (Array.isArray(option.series) ? option.series : []).filter(
    (item): item is LineSeriesOption => item.type === 'line',
  );
  return (
    <Spin loading={loading} style={{ width: '100%' }}>
      <ChartView
        height={300}
        option={{
          ...option,
          tooltip: {
            trigger: 'axis',
            axisPointer: {
              type: 'cross',
              label: { show: false },
              lineStyle: { color: '#C9CDD4', type: 'dashed' },
              crossStyle: { color: '#C9CDD4', type: 'dashed' },
            },
          },
          xAxis: {
            ...option.xAxis,
            splitLine: { show: true, lineStyle: { color: '#E5E8EF' } },
          },
          grid: { top: 10, bottom: 50, left: 45, right: 20 },
          yAxis: {
            ...option.yAxis,
            axisLabel: { fontSize: 10, formatter: formatThousands },
            splitLine: { lineStyle: { type: 'dashed' } },
          },
          series: series.map((item) => ({
            ...item,
            name,
            symbol: 'circle',
            showSymbol: false,
            symbolSize: 16,
            emphasis: { scale: false, itemStyle: { color: '#fff' } },
            lineStyle: {
              width: 3,
              color: {
                type: 'linear',
                x: 0,
                y: 0,
                x2: 1,
                y2: 0,
                colorStops: [
                  { offset: 0, color: '#1EE7FF' },
                  { offset: 0.57, color: '#249AFF' },
                  { offset: 1, color: '#6F42FB' },
                ],
              },
            },
            areaStyle: {
              opacity: 1,
              color: verticalGradient(
                'rgba(17,126,255,0.5)',
                'rgba(17,128,255,0)',
              ),
            },
            itemStyle: { color, borderColor: color, borderWidth: 3 },
          })),
        }}
      />
    </Spin>
  );
}
