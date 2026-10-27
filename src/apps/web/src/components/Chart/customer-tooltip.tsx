import React from 'react';
import type { TooltipComponentFormatterCallbackParams } from 'echarts';
import { Typography, Badge } from '@arco-design/web-react';
import styles from './style/index.module.less';

const { Text } = Typography;
interface TooltipProps {
  title: string;
  data: {
    name: string;
    value: string;
    color: string;
  }[];
  color?: string;
  name?: string;
  formatter?: (value: string) => React.ReactNode;
}

function CustomTooltip(props: TooltipProps) {
  const { formatter = (value) => value, color, name } = props;
  return (
    <div className={styles['customer-tooltip']}>
      <div className={styles['customer-tooltip-title']}>
        <Text bold>{props.title}</Text>
      </div>
      <div>
        {props.data.map((item, index) => (
          <div className={styles['customer-tooltip-item']} key={index}>
            <div>
              <Badge color={color || item.color} />
              {name || item.name}
            </div>
            <div>
              <Text bold>{formatter(item.value)}</Text>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export default CustomTooltip;

// ECharts owns the floating container; build its contents with DOM text nodes so
// API-provided labels are never interpolated into HTML.
export function formatChartTooltip(
  params: TooltipComponentFormatterCallbackParams,
) {
  const items = Array.isArray(params) ? params : [params];
  const root = document.createElement('div');
  root.className = styles['customer-tooltip'];
  const first = items[0];
  const title = document.createElement('div');
  title.className = styles['customer-tooltip-title'];
  title.textContent =
    first && 'axisValueLabel' in first
      ? String(first.axisValueLabel)
      : first?.name || '';
  root.append(title);
  items.forEach((item) => {
    const row = document.createElement('div');
    row.className = styles['customer-tooltip-item'];
    const label = document.createElement('span');
    const marker = document.createElement('i');
    marker.className = styles['customer-tooltip-marker'];
    marker.style.backgroundColor =
      typeof item.color === 'string' ? item.color : '#4080FF';
    label.append(
      marker,
      document.createTextNode(item.seriesName || item.name || ''),
    );
    const value = document.createElement('span');
    value.className = styles['customer-tooltip-value'];
    const raw = Array.isArray(item.value)
      ? item.value[item.value.length - 1]
      : item.value;
    value.textContent =
      typeof raw === 'number' ? raw.toLocaleString() : String(raw ?? '');
    row.append(label, value);
    root.append(row);
  });
  return root;
}
