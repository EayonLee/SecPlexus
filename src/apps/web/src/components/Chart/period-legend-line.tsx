import { Spin } from '@arco-design/web-react';
import ChartView from './chart-view';
import {
  cartesian,
  linePalette,
  verticalGradient,
  type ChartRow,
} from './options';

export default function PeriodLine({
  data,
  loading,
}: {
  data: ChartRow[];
  loading: boolean;
}) {
  return (
    <Spin loading={loading} style={{ width: '100%' }}>
      <ChartView
        height={370}
        option={{
          ...cartesian(data, 'time', 'rate', 'name'),
          color: linePalette,
          yAxis: {
            type: 'value',
            min: 0,
            max: 100,
            interval: 25,
            axisLabel: { fontSize: 10, formatter: '{value} %' },
          },
          legend: {
            bottom: 48,
            icon: 'circle',
            itemWidth: 8,
            itemHeight: 8,
            textStyle: { fontSize: 10 },
          },
          dataZoom: [
            {
              type: 'slider',
              bottom: 8,
              height: 20,
              borderColor: 'transparent',
              fillerColor: 'rgba(146,186,255,0.25)',
              dataBackground: {
                areaStyle: { color: 'rgba(4,135,255,0.15)' },
                lineStyle: { color: 'rgba(36,158,255,0.3)' },
              },
              selectedDataBackground: {
                areaStyle: {
                  color: verticalGradient(
                    'rgba(206,224,255,0.9)',
                    'rgba(146,186,255,0.8)',
                  ),
                },
              },
              handleStyle: { color: '#fff', borderColor: '#165DFF' },
            },
          ],
          grid: { top: 10, bottom: 100, left: 60, right: 20 },
        }}
      />
    </Spin>
  );
}
