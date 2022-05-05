load("rule.star", "attr", "rule")

def _tar_impl(ctx):
    # TODO: providers list?
    return ctx.actions.packaging.tar(
        name = ctx.attrs.name,
        strip_prefix = ctx.attrs.strip_prefix,
        package_dir = ctx.attrs.package_dir,
        srcs = ctx.attrs.srcs,
    )

tar = rule(
    impl = _tar_impl,
    attrs = {
        "strip_prefix": attr.string(),
        "package_dir": attr.string(default = "/"),
        "srcs": attr.label_list(mandatory = True),
    },
)