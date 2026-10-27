import React, { useState, useEffect } from 'react';
import { Card, Spin, Typography } from '@arco-design/web-react';
import ChartView from '@/components/Chart/chart-view';
import { donut } from '@/components/Chart/options';
import axios from 'axios';
import useLocale from '@/utils/useLocale';
import locale from './locale';

function PopularContent() {
  const t = useLocale(locale);
  const [data, setData] = useState([]);
  const [loading, setLoading] = useState(true);

  const fetchData = () => {
    setLoading(true);
    axios
      .get('/api/workplace/content-percentage')
      .then((res) => {
        setData(res.data);
      })
      .finally(() => {
        setLoading(false);
      });
  };

  useEffect(() => {
    fetchData();
  }, []);

  return (
    <Card>
      <Typography.Title heading={6}>
        {t['workplace.contentPercentage']}
      </Typography.Title>
      <Spin loading={loading} style={{ display: 'block' }}>
        <ChartView height={340} option={donut(data)} />
      </Spin>
    </Card>
  );
}

export default PopularContent;
