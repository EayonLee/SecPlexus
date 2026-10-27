import type { TooltipComponentOption } from 'echarts';
import React, { useEffect, useState, useMemo } from 'react';
import {
  Statistic,
  Typography,
  Spin,
  Grid,
  Card,
  Skeleton,
} from '@arco-design/web-react';
import cs from 'classnames';
import ChartView from '@/components/Chart/chart-view';
import { cartesian, donut } from '@/components/Chart/options';
import axios from 'axios';
import useLocale from '@/utils/useLocale';
import locale from './locale';

import { IconArrowRise, IconArrowFall } from '@arco-design/web-react/icon';
import styles from './style/card-block.module.less';

const { Row, Col } = Grid;
const { Title, Text } = Typography;
export interface CardProps {
  key: string;
  title?: string;
  chartData?: any[];
  chartType: string;
  count?: number;
  increment?: boolean;
  diff?: number;
  loading?: boolean;
}

const compactTooltip: TooltipComponentOption = {
  show: true,
  trigger: 'item',
  formatter(params) {
    const item = Array.isArray(params) ? params[0] : params;
    const value = Array.isArray(item.value)
      ? item.value[item.value.length - 1]
      : item.value;
    return Number(value).toLocaleString();
  },
};
function SimpleLine({ chartData = [] }: { chartData: any[] }) {
  return (
    <ChartView
      height={80}
      option={{
        ...cartesian(chartData, 'x', 'y', 'name', 'line', false, true),
        color: ['#165DFF', 'rgba(106,161,255,0.3)'],
        tooltip: compactTooltip,
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
        tooltip: compactTooltip,
        series: [
          {
            type: 'bar',
            barMaxWidth: 16,
            data: chartData.map((row, index) => ({
              value: row.y,
              itemStyle: { color: index % 2 === 0 ? '#86DF6C' : '#468DFF' },
            })),
          },
        ],
      }}
    />
  );
}

function CardBlock(props: CardProps) {
  const { chartType, title, count, increment, diff, chartData, loading } =
    props;

  return (
    <Card className={styles.card}>
      <div className={styles.statistic}>
        <Statistic
          title={
            <Title heading={6} className={styles.title}>
              {title}
            </Title>
          }
          loading={loading}
          value={count}
          extra={
            <div className={styles['compare-yesterday']}>
              {loading ? (
                <Skeleton
                  text={{ rows: 1 }}
                  style={{ width: '100px' }}
                  animation
                />
              ) : (
                <span
                  className={cs(styles['diff'], {
                    [styles['diff-increment']]: increment,
                  })}
                >
                  {diff}
                  {increment ? <IconArrowRise /> : <IconArrowFall />}
                </span>
              )}
            </div>
          }
          groupSeparator
        />
      </div>
      <div className={styles.chart}>
        <Spin style={{ width: '100%' }} loading={loading}>
          {chartType === 'interval' && <SimpleInterval chartData={chartData} />}
          {chartType === 'line' && <SimpleLine chartData={chartData} />}
        </Spin>
      </div>
    </Card>
  );
}

const cardInfo = [
  {
    key: 'userRetentionTrend',
    type: 'line',
  },
  {
    key: 'userRetention',
    type: 'interval',
  },
  {
    key: 'contentConsumptionTrend',
    type: 'line',
  },
  {
    key: 'contentConsumption',
    type: 'interval',
  },
];
function CardList() {
  const t = useLocale(locale);
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState(
    cardInfo.map((item) => ({
      ...item,
      chartType: item.type,
    })),
  );

  const getData = async () => {
    const requestList = cardInfo.map(async (info) => {
      const { data } = await axios
        .get(`/api/multi-dimension/card?type=${info.type}`)
        .catch(() => ({ data: {} }));
      return {
        ...data,
        key: info.key,
        chartType: info.type,
      };
    });

    setLoading(true);
    const result = await Promise.all(requestList).finally(() =>
      setLoading(false),
    );
    setData(result);
  };

  useEffect(() => {
    getData();
  }, []);

  const formatData = useMemo(() => {
    return data.map((item) => ({
      ...item,
      title: t[`multiDAnalysis.cardList.${item.key}`],
    }));
  }, [t, data]);

  return (
    <Row gutter={16}>
      {formatData.map(({ key, ...item }, index) => (
        <Col span={6} key={index}>
          <CardBlock key={key} {...item} loading={loading} />
        </Col>
      ))}
    </Row>
  );
}

export default CardList;
