import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'installation',
    'architecture',
    'pixi-essentials',
    'cli-guide',
    'server-setup',
    'cli-reference',
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
