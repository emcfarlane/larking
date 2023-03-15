load("rule.star", "attr", "label", "rule")

def test_hello_rule(t):
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
    print("rule", rule)

    # Declare test rule.
    hello(
        name = "HelloInput",
        input = "Edward",
    )

    # TODO: expose builder?
    print(label("HelloInput"))
