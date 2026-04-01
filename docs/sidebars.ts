import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'introduction',
    'installation',
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
    {
      type: 'category',
      label: 'Nebi Server (for Team)',
      items: [
        'server-overview',
        'server-setup',
        'registry-setup',
      ],
    },
    {
      type: 'category',
      label: 'Examples',
      items: [
        'examples/sharing-environments',
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
