## open

`open(url, **kwargs)` opens an OpenAPI spec creating a new openAPI client.

| Parameter | Description |
| ------------- | ------------- |
| url | string <br /> Runtimevar URL of openAPI spec. |
| client | http.client <br /> Optional HTTP client to use for all requests. |

### client·service.method

`c.<service>.<method>(**kwargs)` sends a HTTP request and returns a HTTP response.
OpenAPI clients will translate the spec into a typed client.

TODO: examples.
