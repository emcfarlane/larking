## open

`open(url, **kwargs)` opens an OpenAPI spec creating a new openAPI client.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| url | string | Runtimevar URL of openAPI spec. |
| client | http.client | Optional HTTP client to use for all requests. |

### client·service·method

`c.<service>.<method>(**parameters)` sends a HTTP request and returns a HTTP response.
OpenAPI clients will translate the spec into a typed client.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| service | attr | Methods grouped by "tag". |
| method | attr | Callable method in the group. |
| parameters | {}any | Request parameters. |
