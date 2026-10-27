import { Spin } from '@arco-design/web-react';
import ChartView from './chart-view';
import { linePalette, type ChartRow } from './options';

export default function AreaPolar({
  data = [],
  loading,
  fields,
  height = 400,
}: {
  data: ChartRow[];
  loading: boolean;
  fields: string[];
  height: number;
}) {
  return (
    <Spin loading={loading} style={{ width: '100%' }}>
      <ChartView
        height={height}
        option={{
          color: [linePalette[1], linePalette[0], linePalette[2]],
          tooltip: {},
          legend: {
            data: fields,
            right: 0,
            top: 'middle',
            orient: 'vertical',
            icon: 'circle',
            itemWidth: 8,
            itemHeight: 8,
            textStyle: { fontSize: 10 },
          },
          radar: {
            center: ['40%', '50%'],
            startAngle: 90,
            radius: '70%',
            splitNumber: 4,
            splitArea: { show: false },
            axisLine: { lineStyle: { color: '#E5E6EB' } },
            splitLine: { lineStyle: { color: '#E5E6EB' } },
            axisName: { fontSize: 10 },
            indicator: data.map((row) => ({
              name: String(row.item),
              max: 80,
            })),
          },
          series: [
            {
              type: 'radar',
              symbol: 'circle',
              symbolSize: 6,
              lineStyle: { width: 2 },
              data: fields.map((field) => ({
                name: field,
                value: data.map((row) => Number(row[field])),
                areaStyle: { opacity: 0.4 },
              })),
            },
          ],
        }}
      />
    </Spin>
  );
}
