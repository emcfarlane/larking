load("rule.star", "rule", "attrs", "attr", "label", "provides")

def _exec_impl(name, dir="", args=[], envs=[]):
    """Test rule takes name as input and returns a string output."""

    load("@std", "os")
    print("exec")

    os.exec(
        name = name, 
        dir = dir,
        args = args,
        envs = envs,
    )
    return [rsp]  # returns larking.api.SQLQueryResponse


exec = rule(
    impl = _exec_impl,
    attrs = attrs(
        database = attr.string(),
        statement = attr.string(),
        args = attr.list(val_kind = "any", optional=True),
    ),
    provides = provides(
        attr.message("larking.api.SQLQueryInfo"),
    ),
)
