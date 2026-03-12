import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'introduction',
    'installation',
    {
      type: 'category',
      label: 'CLI Usage',
      items: [
        'cli-local',
        'cli-team',
        'cli-reference',
      ],
    },
    'desktop-overview',
    {
      type: 'category',
      label: 'Nebi Server (for Team)',
      items: [
        'server-setup',
      ],
    },
    {
      type: 'category',
      label: 'Maintainers',
      items: [
        'maintainers-conda-forge',
      ],
    },
  ],
};

export default sidebars;
