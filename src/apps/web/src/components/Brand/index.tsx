import styles from './style.module.less';

export default function Brand({ className = '' }: { className?: string }) {
  return <span className={`${styles.wordmark} ${className}`}>SecPlexus</span>;
}
