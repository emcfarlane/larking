## default_client

`default_client` is a constant for the default client implementation.

### client·do

`c.do(req)` sends the HTTP request and returns an HTTP response, as per the client.

| Parameter | Description |
| ------------- | ------------- |
| req | http.request <br /> Request to send. |

## get

`get(url)` sends a GET requests and returns an HTTP response with the default client.

| Parameter | Description |
| ------------- | ------------- |
| url | string <br /> URL of request. |

## head

`head(url)` sends a HEAD request to the URL and returns an HTTP response.

| Parameter | Description |
| ------------- | ------------- |
| url | string <br /> URL of request. |

## post

`post(url, body)` sends a POST request to the URL with the provided body and returns an HTTP response.

| Parameter | Description |
| ------------- | ------------- |
| url | string <br /> URL of request. |
| body | string, bytes, io.reader <br /> Body of request. |

## new_client

`new_client()` TODO

## new_request

`new_request(method, url, body)`

| Parameter | Description |
| ------------- | ------------- |
| method | string <br /> HTTP method of request. |
| url | string <br /> URL of request. |
| body | string, bytes, io.reader <br /> Body of request. |

## errors

- `err_not_supported`
- `err_missing_boundary`
- `err_not_multipart`
- `err_body_not_allowed`
- `err_hijacked`
- `err_content_length`
- `err_abort_handler`
- `err_body_read_after_close`
- `err_handler_timeout`
- `err_line_too_long`
- `err_missing_file`
- `err_no_cookie`
- `err_no_location`
- `err_server_closed`
- `err_skip_alt_protocol`
- `err_use_last_response`
