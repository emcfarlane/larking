load("rule.star", "DefaultInfo", "attr", "attrs", "rule")

def _tar_impl(ctx):
    # TODO: build list of files from DefaultInfo.

    out = ctx.actions.archive.tar(
        name = ctx.attrs.name,
        strip_prefix = ctx.attrs.strip_prefix,
        package_dir = ctx.attrs.package_dir,
        srcs = ctx.attrs.srcs,
    )
    return [DefaultInfo(
        files = [out],
    )]

tar = rule(
    impl = _tar_impl,
    attrs = attrs(
        strip_prefix = attr.string(),
        package_dir = attr.string(default = "/"),
        srcs = attr.label_list(mandatory = True),
    ),
    provides = [DefaultInfo],
)
