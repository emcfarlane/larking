module.exports = {
  title: "GraphPB",
  tagline: "Reflective protobuffer APIs",
  url: "https://emcfarlane.github.io",
  baseUrl: "/graphpb/",
  favicon: "img/favicon.ico",
  organizationName: "emcfarlane",
  projectName: "graphpb",
  themeConfig: {
    navbar: {
      title: "GraphPB",
      logo: {
        alt: "GraphPB",
        src: "img/logo_one.svg",
      },
      links: [
        {
          to: "docs/intro",
          activeBasePath: "intro",
          label: "Docs",
          position: "left",
        },
        {
          href: "https://github.com/emcfarlane/graphpb",
          position: "right",
          className: ["header-github", "header-logo"],
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
