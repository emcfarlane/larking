module.exports = {
  title: "Larking",
  tagline: "Reflective protobuffer APIs",
  url: "https://emcfarlane.github.io",
  baseUrl: "/larking/",
  favicon: "img/favicon.ico",
  organizationName: "emcfarlane",
  projectName: "larking",
  themeConfig: {
    navbar: {
      title: "Larking",
      logo: {
        alt: "Larking",
        src: "img/logo_one.svg",
      },
      items: [
        {
          to: "docs/intro",
          activeBasePath: "intro",
          label: "Docs",
          position: "left",
        },
        {
          href: "https://github.com/emcfarlane/larking",
          position: "right",
          className: "header-github header-logo",
        },
      ],
    },
    footer: {
      style: "dark",
      copyright: `Copyright © ${new Date().getFullYear()} Edward McFarlane. Built with Docusaurus.`,
    },
  },
  presets: [
    [
      "@docusaurus/preset-classic",
      {
        docs: {
          sidebarPath: require.resolve("./sidebars.js"),
          editUrl:
            "https://github.com/facebook/docusaurus/edit/master/website/",
        },
        theme: {
          customCss: require.resolve("./src/css/custom.css"),
        },
      },
    ],
  ],
};
