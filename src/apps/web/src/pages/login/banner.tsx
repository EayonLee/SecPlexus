import React from 'react';
import operations from './assets/operations.svg?url';
import protection from './assets/agent-protection.svg?url';
import evaluation from './assets/model-evaluation.svg?url';
import { Carousel } from '@arco-design/web-react';
import useLocale from '@/utils/useLocale';
import locale from './locale';
import styles from './style/index.module.less';

export default function LoginBanner() {
  const t = useLocale(locale);
  const data = [
    {
      slogan: t['login.banner.slogan1'],
      subSlogan: t['login.banner.subSlogan1'],
      image: operations,
    },
    {
      slogan: t['login.banner.slogan2'],
      subSlogan: t['login.banner.subSlogan2'],
      image: protection,
    },
    {
      slogan: t['login.banner.slogan3'],
      subSlogan: t['login.banner.subSlogan3'],
      image: evaluation,
    },
  ];
  return (
    <Carousel className={styles.carousel} animation="fade">
      {data.map((item, index) => (
        <div key={`${index}`}>
          <div className={styles['carousel-item']}>
            <div className={styles['carousel-title']}>{item.slogan}</div>
            <div className={styles['carousel-sub-title']}>{item.subSlogan}</div>
            <img
              alt={item.slogan}
              className={styles['carousel-image']}
              src={item.image}
            />
          </div>
        </div>
      ))}
    </Carousel>
  );
}
