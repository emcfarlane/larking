import React from "react";
import classnames from "classnames";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import useBaseUrl from "@docusaurus/useBaseUrl";
import styles from "./styles.module.css";

const features = [
  {
    title: <>gRPC First</>,
    imageUrl: "img/grpc-icon-color.png",
    description: (
      <>
        Supports any gRPC services implementing reflection. Protobuffer
        introspection dynamically reloeads on new server deployments.
      </>
    ),
  },
  {
    title: <>Transcoding HTTP/JSON to gRPC</>,
    imageUrl: "img/rest.png",
    description: (
      <>
        GraphPB lets you focus on API specification. REST bindings are generated
        directily from protobuffer descriptors with no need for more code
        generation.
      </>
    ),
  },
  {
    title: <>Powered by Go</>,
    imageUrl: "img/gopher.png",
    description: (
      <>Written in Go. Extend or customize with starlark scripting.</>
    ),
  },
];

function Feature({ imageUrl, title, description }) {
  const imgUrl = useBaseUrl(imageUrl);
  return (
    <div className={classnames("col col--4", styles.feature)}>
      {imgUrl && (
        <div className={classnames("text--center", styles.featureImage)}>
          <img src={imgUrl} alt={title} />
        </div>
      )}
      <h3>{title}</h3>
      <p>{description}</p>
    </div>
  );
}
//<!--<h1 className="hero__title">{siteConfig.title}</h1>
//<p className="hero__subtitle">{siteConfig.tagline}</p>!-->
//<div className={styles.buttons}>
//  <Link
//    className={classnames(
//      "button button--outline button--secondary button--lg",
//      styles.getStarted
//    )}
//    to={useBaseUrl("docs/doc1")}
//  >
//    Get Started
//  </Link>
//</div>

function Home() {
  const context = useDocusaurusContext();
  const { siteConfig = {} } = context;
  return (
    <Layout title={`${siteConfig.title}`} description="WIP">
      <header className="hero">
        <div className="container">
          <div className="row">
            <div className="col">
              <div className="heroTagline">Reflective protobuffer APIs</div>
              <Link
                className="button button--outline button--secondary button--lg"
                to={useBaseUrl("docs/doc1")}
              >
                Get Started
              </Link>
            </div>
            <div className="col">
              <img className="heroLogo" src="img/GraphPB_hero.png" />
            </div>
          </div>
        </div>
      </header>
      <main>
        {features && features.length && (
          <section className={styles.features}>
            <div className="container">
              <div className="row">
                {features.map((props, idx) => (
                  <Feature key={idx} {...props} />
                ))}
              </div>
            </div>
          </section>
        )}
      </main>
    </Layout>
  );
}

export default Home;
