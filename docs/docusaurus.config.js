module.exports = {
  title : 'graphpb',
  tagline : 'Reflective protobuffer APIs',
  url : 'https://emcfarlane.github.io',
  baseUrl : '/graphpb/',
  favicon : 'img/favicon.ico',
  organizationName : 'emcfarlane',
  projectName : 'graphpb',
  themeConfig : {
    navbar : {
      title : 'graphpb',
      logo : {
        alt : 'graphpb',
        src : 'img/logo_one.svg',
      },
      links : [
        {
          to : 'docs/doc1',
          activeBasePath : 'docs',
          label : 'Docs',
          position : 'right',
        },
        {
          href : 'https://github.com/emcfarlane/graphpb',
          label : 'GitHub',
          position : 'right',
        },
      ],
    },
    footer : {
      style : 'dark',
      links : [
        {
          title : 'Docs',
          items : [
            {
              label : 'Style Guide',
              to : 'docs/doc1',
            },
            {
              label : 'Second Doc',
              to : 'docs/doc2',
            },
          ],
        },
      ],
      copyright : `Copyright © ${
          new Date().getFullYear()} Edward McFarlane. Built with Docusaurus.`,
    },
  },
  presets : [
    [
      '@docusaurus/preset-classic',
      {
        docs : {
          sidebarPath : require.resolve('./sidebars.js'),
          editUrl :
              'https://github.com/facebook/docusaurus/edit/master/website/',
        },
        theme : {
          customCss : require.resolve('./src/css/custom.css'),
        },
      },
    ],
  ],
};
