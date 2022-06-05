load("rule.star", "DefaultInfo", "attr", "attrs", "rule")

def _tar_impl(ctx):
    files = set([file for src in ctx.attrs.srcs for file in src[DefaultInfo].files])
    print("have files: ", files)
    srcs = [src for src in files]  # to list, should be set?

    out = ctx.actions.archive.tar(
        name = ctx.attrs.name,
        strip_prefix = ctx.attrs.strip_prefix,
        package_dir = ctx.attrs.package_dir,
        srcs = srcs,
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
