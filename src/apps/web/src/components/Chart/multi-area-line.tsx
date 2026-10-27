import { Spin } from '@arco-design/web-react';
import ChartView from './chart-view';
import { cartesian, verticalGradient, type ChartRow } from './options';

export default function MultiAreaLine({
  data,
  loading,
}: {
  data: ChartRow[];
  loading: boolean;
}) {
  const option = cartesian(data, 'time', 'count', 'name', 'line', true);
  const gradients: [string, string][] = [
    ['rgba(131,100,255,0.5)', 'rgba(80,52,255,0.001)'],
    ['rgba(100,255,236,0.5)', 'rgba(52,255,243,0.001)'],
    ['rgba(255,211,100,0.5)', 'rgba(255,235,52,0.001)'],
    ['rgba(100,162,255,0.5)', 'rgba(52,105,255,0.001)'],
  ];
  const series = Array.isArray(option.series) ? option.series : [];
  return (
    <Spin loading={loading} style={{ width: '100%' }}>
      <ChartView
        height={352}
        option={{
          ...option,
          color: ['#722ED1', '#33D1C9', '#F77234', '#165DFF'],
          legend: { show: false },
          grid: { top: 10, bottom: 30, left: 40, right: 0 },
          yAxis: {
            type: 'value',
            axisLine: { show: false },
            splitLine: { lineStyle: { type: 'solid' } },
            splitNumber: 4,
            max: (value) => Math.ceil(value.max / 1000) * 1000,
            axisLabel: {
              fontSize: 10,
              formatter: (value: string | number) => `${Number(value) / 100} k`,
            },
          },
          series: series.map((item, index) => ({
            ...item,
            areaStyle: {
              opacity: 1,
              color: verticalGradient(...gradients[index % gradients.length]),
            },
          })),
        }}
      />
    </Spin>
  );
}
