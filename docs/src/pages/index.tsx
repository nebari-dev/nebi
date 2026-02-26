import type {ReactNode} from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={styles.heroBanner}>
      <div className="container">
        <img
          src="/img/nebi-icon.svg"
          alt="Nebi"
          className={styles.heroLogo}
        />
        <Heading as="h1" className={styles.heroTitle}>
          {siteConfig.title}
        </Heading>
        <p className={styles.heroSubtitle}>{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link
            className={styles.heroButton}
            to="/docs/getting-started">
            Get Started
          </Link>
        </div>
      </div>
    </header>
  );
}

function HomepageDemo() {
  return (
    <section className={styles.demoSection}>
      <div className="container">
        <Heading as="h2" className={styles.demoHeading}>
          See Nebi in Action
        </Heading>
        <div className={styles.demoGrid}>
          <div className={styles.demoItem}>
            <Heading as="h3" className={styles.demoLabel}>Web UI</Heading>
            <img
              src="https://raw.githubusercontent.com/nebari-dev/nebi-video-demo-automation/25e0139cf70cc0e9f8a2cf938fddd85ecd83adee/assets/demo.gif"
              alt="Nebi Web UI Demo"
              className={styles.demoImage}
              loading="lazy"
            />
          </div>
          <div className={styles.demoItem}>
            <Heading as="h3" className={styles.demoLabel}>CLI</Heading>
            <img
              src="/nebi-demo.gif"
              alt="Nebi CLI Demo"
              className={styles.demoImage}
              loading="lazy"
            />
          </div>
        </div>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  return (
    <Layout
      title="Home"
      description="Nebi - Multi-user environment management for Pixi">
      <HomepageHeader />
      <main>
        <HomepageDemo />
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
