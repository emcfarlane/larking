## error

`error(*args, **kwargs)` creates a new error.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| *args | []any | Args like `print`. |
| **kwargs | {}any | Kwargs like `print`. |

### error·matches

`e.matches(pattern)` returns `True` if the regex pattern matches the error string.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| pattern | string | Regex pattern to match. |

### error·kind

`e.kind(err)` returns `True` if the error is of the same kind as `err`.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| err | error | Error to compare against. |


## catch

`catch(fn, *args, **kwargs)`
evaluates the provided function and returns a result.
Catch allows starlark code to capture errors returned from a function call.
The `function` must be a callable that accepts the remaining args and kwargs passed to catch.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| fn | function | Function to call. |
| *args | []any | Args for `fn`. |
| **kwargs | {}any | Kwargs for `fn`. |
