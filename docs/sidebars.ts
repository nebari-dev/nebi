import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'introduction',
    'installation',
    'quick-start',
    'nebi-components',
    'pixi-essentials',
    {
      type: 'category',
      label: 'CLI Usage',
      items: [
        'cli-local',
        'cli-team',
        'cli-reference',
      ],
    },
    'desktop',
    'registry-setup',
    'server-setup',
    {
      type: 'category',
      label: 'Examples',
      items: [
        'examples/sharing-environments',
        'examples/version-rollback',
      ],
    },
    {
      type: 'category',
      label: 'Development',
      items: [
        'maintainers-conda-forge',
      ],
    },
  ],
};

export default sidebars;
