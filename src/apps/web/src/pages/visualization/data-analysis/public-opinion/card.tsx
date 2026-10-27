import React from 'react';
import { Skeleton, Statistic, Typography } from '@arco-design/web-react';
import cs from 'classnames';
import ChartView from '@/components/Chart/chart-view';
import { cartesian, donut } from '@/components/Chart/options';

import { IconArrowRise, IconArrowFall } from '@arco-design/web-react/icon';
import styles from '../style/public-opinion.module.less';

const { Title, Text } = Typography;
export interface PublicOpinionCardProps {
  key: string;
  title: string;
  chartData?: any[];
  chartType: 'line' | 'interval' | 'pie';
  count?: number;
  increment?: boolean;
  diff?: number;
  compareTime?: string;
  loading?: boolean;
}

function SimpleLine({ chartData = [] }: { chartData: any[] }) {
  return (
    <ChartView
      height={80}
      option={{
        ...cartesian(chartData, 'x', 'y', 'name', 'line', false, true),
        color: ['#165DFF', 'rgba(106,161,255,0.3)'],
        series: ['类目1', '类目2'].map((name, index) => ({
          name,
          type: 'line',
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 3, type: index === 1 ? 'dashed' : 'solid' },
          data: chartData
            .filter((row) => row.name === name)
            .map((row) => [row.x, row.y]),
        })),
      }}
    />
  );
}
function SimpleInterval({ chartData = [] }: { chartData: any[] }) {
  return (
    <ChartView
      height={80}
      option={{
        ...cartesian(chartData, 'x', 'y', undefined, 'bar', false, true),
        series: [
          {
            type: 'bar',
            barMaxWidth: 14,
            data: chartData.map((row, index) => ({
              value: row.y,
              itemStyle: {
                color: index % 2 === 0 ? '#2CAB40' : '#86DF6C',
                borderRadius: 7,
              },
            })),
          },
        ],
      }}
    />
  );
}
function SimplePie({ chartData = [] }: { chartData: any[] }) {
  return (
    <ChartView height={80} option={donut(chartData, 'name', 'count', true)} />
  );
}

function PublicOpinionCard(props: PublicOpinionCardProps) {
  const { chartType, title, count, increment, diff, chartData, loading } =
    props;
  const className = cs(styles.card, styles[`card-${chartType}`]);

  return (
    <div className={className}>
      <div className={styles.statistic}>
        <Statistic
          title={
            <Title heading={6} className={styles.title}>
              {title}
            </Title>
          }
          loading={loading}
          value={count}
          groupSeparator
        />
        <div className={styles['compare-yesterday']}>
          <Text type="secondary" className={styles['compare-yesterday-text']}>
            {props.compareTime}
          </Text>
          <span
            className={cs(styles['diff'], {
              [styles['diff-increment']]: increment,
            })}
          >
            {loading ? (
              <Skeleton text={{ rows: 1 }} animation />
            ) : (
              <>
                {diff}
                {increment ? <IconArrowRise /> : <IconArrowFall />}
              </>
            )}
          </span>
        </div>
      </div>
      <div className={styles.chart}>
        {loading ? (
          <Skeleton
            text={{ rows: 3, width: Array(3).fill('100%') }}
            animation
          />
        ) : (
          <>
            {chartType === 'interval' && (
              <SimpleInterval chartData={chartData} />
            )}
            {chartType === 'line' && <SimpleLine chartData={chartData} />}
            {chartType === 'pie' && <SimplePie chartData={chartData} />}
          </>
        )}
      </div>
    </div>
  );
}

export default PublicOpinionCard;
