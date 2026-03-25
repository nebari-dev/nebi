import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'installation',
    'introduction',
    'architecture',
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
    {
      type: 'category',
      label: 'Nebi Server (for Team)',
      items: [
        'server-setup',
        'server-overview',
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
