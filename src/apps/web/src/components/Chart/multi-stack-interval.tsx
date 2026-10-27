import { Spin } from '@arco-design/web-react';
import ChartView from './chart-view';
import {
  cartesian,
  publishingPalette,
  formatThousands,
  type ChartRow,
} from './options';

export default function MultiInterval({
  data,
  loading,
}: {
  data: ChartRow[];
  loading: boolean;
}) {
  const option = cartesian(
    data,
    'time',
    'count',
    'name',
    'bar',
    false,
    false,
    false,
    true,
  );
  const series = (Array.isArray(option.series) ? option.series : []).map(
    (item, index) => ({
      ...item,
      itemStyle: {
        color: publishingPalette[index],
        borderRadius: [2, 2, 0, 0],
      },
    }),
  );
  return (
    <Spin loading={loading} style={{ width: '100%' }}>
      <ChartView
        height={370}
        option={{
          ...option,
          color: publishingPalette,
          legend: {
            ...option.legend,
            data: series.map((item) => String(item.name)),
          },
          yAxis: {
            ...option.yAxis,
            axisLabel: { fontSize: 10, formatter: formatThousands },
          },
          series: series.reverse(),
        }}
      />
    </Spin>
  );
}
