import { Spin } from '@arco-design/web-react';
import ChartView from './chart-view';
import { palette, type ChartRow } from './options';

export default function FactMultiPie({
  data = [],
  loading,
  height = 400,
}: {
  data: ChartRow[];
  loading: boolean;
  height: number;
}) {
  const groups = [...new Set(data.map((row) => String(row.category)))];
  return (
    <Spin loading={loading} style={{ width: '100%' }}>
      <ChartView
        height={height}
        option={{
          color: palette,
          tooltip: { trigger: 'item' },
          legend: {
            bottom: 0,
            icon: 'circle',
            itemWidth: 8,
            itemHeight: 8,
            textStyle: { fontSize: 10 },
          },
          title: groups.map((category, index) => ({
            text: category,
            left: `${((index + 0.5) * 100) / groups.length}%`,
            top: '42%',
            textAlign: 'center',
            textStyle: { fontSize: 14, fontWeight: 500 },
          })),
          series: groups.map((category, index) => ({
            type: 'pie',
            name: category,
            radius: ['56%', '80%'],
            left: `${(index * 100) / groups.length}%`,
            width: `${100 / groups.length}%`,
            top: 0,
            bottom: 14,
            center: ['50%', '49%'],
            label: {
              show: true,
              formatter: (item) => `${Number(item.percent).toFixed(2)} %`,
              fontSize: 10,
            },
            labelLine: { length: 12, length2: 28 },
            emphasis: { scale: false },
            selectedMode: 'single',
            data: data
              .filter((row) => row.category === category)
              .map((row) => ({
                name: String(row.type),
                value: Number(row.value),
              })),
          })),
        }}
      />
    </Spin>
  );
}
