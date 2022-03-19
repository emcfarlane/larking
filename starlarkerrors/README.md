## error

`error(*args, **kwargs)` creates a new error.

| Parameter | Description |
| ------------- | ------------- |
| *args | [] <br /> Args like `print`. |
| **kwargs | {} <br /> Kwargs like `print`. |

### error·matches

`e.matches(pattern)` returns `True` if the regex pattern matches the error string.

| Parameter | Description |
| ------------- | ------------- |
| pattern | string <br /> Regex pattern to match. |

### error·kind

`e.kind(err)` returns `True` if the error is of the same kind as `err`.


## catch

`catch(function, *args, **kwargs)`
evaluates the provided function and returns a result.
Catch allows starlark code to capture errors returned from a function call.
The `function` must be a callable that accepts the remaining args and kwargs passed to catch.
