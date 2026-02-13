import type {ReactNode} from 'react';
import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  description: ReactNode;
};

const FeatureList: FeatureItem[] = [
  {
    title: 'Local-First Workspaces',
    description: (
      <>
        Track and manage Pixi workspaces locally. Initialize projects, add
        dependencies, and work offline with full control over your environments.
      </>
    ),
  },
  {
    title: 'Sync and Share',
    description: (
      <>
        Push workspace specs to a Nebi server and pull them on another machine.
        Share reproducible environments with your team using simple version tags.
      </>
    ),
  },
  {
    title: 'Diff and Publish',
    description: (
      <>
        Compare workspace specs across versions or directories. Publish
        environments to OCI registries for distribution and reproducibility.
      </>
    ),
  },
];

function Feature({title, description}: FeatureItem) {
  return (
    <div className={clsx('col col--4')}>
      <div className="text--center padding-horiz--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures(): ReactNode {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
