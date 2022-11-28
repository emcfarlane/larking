load("rule.star", "rule", "attrs", "attr", "label", "provides")

#def test_default_attrs(t):
#    info = DefaultInfo(
#        files = ["source"],
#    )
#
#    print(info)
#    t.eq(len(info.files), 1)
#    file = info.files[0]
#    t.eq(file.key, "source")

def test_hello_rule(t):
    def _hello_impl(name, input):
        """Test rule takes name as input and returns a string output."""
        print("name", name)
        print("input", input)
        msg = "Hello, %s" % input
        return [msg]


    hello = rule(
        impl = _hello_impl,
        attrs = attrs(
            input = attr.string(),
        ),
        provides = provides(
            attr.string(),
        ),
    )
    print("rule", rule)

    # Declare test rule.
    hello(
        name = "HelloInput",
        input = "Edward",
    )

    # TODO: expose builder?
    print(label("HelloInput"))

