import { Spin } from '@arco-design/web-react';
import ChartView from './chart-view';
import { cartesian, formatThousands, type ChartRow } from './options';

export default function HorizontalInterval({
  data,
  loading,
  height = 370,
}: {
  data: ChartRow[];
  loading: boolean;
  height?: number;
}) {
  const option = cartesian(
    data,
    'name',
    'count',
    undefined,
    'bar',
    false,
    false,
    true,
  );
  const series = Array.isArray(option.series) ? option.series : [];
  return (
    <Spin loading={loading} style={{ width: '100%' }}>
      <ChartView
        height={height}
        option={{
          ...option,
          color: ['#4086FF'],
          grid: { top: 10, bottom: 25, left: 60, right: 20 },
          xAxis: {
            ...option.xAxis,
            axisLabel: { fontSize: 10, formatter: formatThousands },
          },
          series: series.map((item) => ({
            ...item,
            barWidth: 10,
            itemStyle: { borderRadius: 5 },
          })),
        }}
      />
    </Spin>
  );
}
