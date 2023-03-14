# starlarkrule

Rules declare a dependency graph.

```python

def _hello_impl(name, input):
    """Test rule takes name as input and returns a string output."""
    print("name", name)
    print("input", input)
    msg = "Hello, %s" % input
    return [msg]


hello = rule(
    impl = _hello_impl,
    attrs = {
        "input": attr.string(),
    },
    provides = [
        attr.string(),
    ],
)

```

