import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'installation',
    'introduction',
    'architecture',
    'pixi-essentials',
    'cli-guide',
    'server-setup',
    'registry-setup',
    'cli-reference',
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
      label: 'Maintainers',
      items: [
        'maintainers-conda-forge',
      ],
    },
  ],
};

export default sidebars;
