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
        Supports gRPC servers that implement reflection. Protobuffer
        introspection dynamically reloads on new server deployments.
      </>
    ),
  },
  {
    title: <>Transcoding HTTP/JSON to gRPC</>,
    imageUrl: "img/rest.png",
    description: (
      <>
        Larking lets you focus on API specification. REST bindings are generated
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

function Home() {
  const context = useDocusaurusContext();
  const { siteConfig = {} } = context;
  return (
    <Layout title={`${siteConfig.title}`} description="WIP">
      <header className="hero">
        <div className="container">
          <div className="row">
            <div className="col">
              <div className="hero-tag-line">Reflective Protobuffer APIs</div>
              <Link
                className="button button--outline button--secondary button--lg"
                to={useBaseUrl("docs/intro")}
              >
                Get Started
              </Link>
            </div>
            <div className="col">
              <img className="hero-logo" src="img/Larking_hero.png" />
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
