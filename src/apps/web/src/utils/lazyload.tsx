import { lazy, Suspense, type ComponentType } from 'react';
import { Spin } from '@arco-design/web-react';
import styles from '../style/layout.module.less';

export default function lazyload(
  loader: () => Promise<{ default: ComponentType }>,
) {
  const Component = lazy(loader);
  return Object.assign(
    function LazyPage() {
      return (
        <Suspense
          fallback={
            <div className={styles.spin}>
              <Spin />
            </div>
          }
        >
          <Component />
        </Suspense>
      );
    },
    { preload: loader },
  );
}
