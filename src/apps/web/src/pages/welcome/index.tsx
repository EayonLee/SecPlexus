import { Card, Typography } from '@arco-design/web-react';
import { useSelector } from 'react-redux';
import type { GlobalState } from '@/store';

export default function Welcome() {
  const userInfo = useSelector((state: GlobalState) => state.userInfo);
  return (
    <Card>
      <Typography.Title heading={5}>SecPlexus</Typography.Title>
      <Typography.Text type="secondary">
        {userInfo?.name}
        {userInfo?.email ? ` · ${userInfo.email}` : ''}
      </Typography.Text>
    </Card>
  );
}
