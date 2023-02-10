load("rule.star", "actions", "attr", "rule")

def _protoc_impl(ctx):
    outs = []
    args = []
    for plugin in ctx.attrs.plugin:
        if p.args:
            args += args

        # TODO: handle outs

    print("here?")
    args.append()

    # Maybe?
    ctx.actions.run(
        name = "protoc",
        args = args,
    )

    # Provider of  outs.
    return struct(
        outs = outs,
    )

protoc = rule(
    impl = _protoc_impl,
    attrs = {
        "plugins": attr.label_list(),  # TODO: provider type.
        "srcs": attr.label_list(allow_files = ["*.proto"]),
    },
)

# protoc plugin could install the plugin?
# go install ?
def _protoc_plugin_impl(ctx):
    # TODO: provider?
    # "--go_out=paths=source_relative:.",
    # exapand strings?
    return struct(args = ctx.attrs.args, outs = ctx.attrs.outs)

protoc_plugin = rule(
    impl = _protoc_plugin_impl,
    attrs = {
        "args": attr.string_list(),
        "deps": attr.label_list(),
    },
)
