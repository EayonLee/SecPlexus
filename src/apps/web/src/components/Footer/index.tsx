import React from 'react';
import { Layout } from '@arco-design/web-react';
import { FooterProps } from '@arco-design/web-react/es/Layout/interface';
import cs from 'classnames';
import styles from './style/index.module.less';

function Footer(props: FooterProps = {}) {
  const { className, ...restProps } = props;
  return (
    <Layout.Footer className={cs(styles.footer, className)} {...restProps}>
      <div>SecPlexus</div>
      <div>Copyright © 2026–2027 secplexus.com All Rights Reserved</div>
    </Layout.Footer>
  );
}

export default Footer;
