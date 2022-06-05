#load("rule.star", "actions", "attr", "rule")
load("rule.star", "ContainerInfo", "DefaultInfo", "attr", "attrs", "rule")

def _container_pull_impl(ctx):
    print("pulling", ctx)
    file = ctx.actions.container.pull(
        name = ctx.attrs.name,
        reference = ctx.attrs.reference,
    )
    print("RETURNING", file)
    return [
        DefaultInfo(files = [file]),
        ContainerInfo(src = file, reference = ctx.attrs.reference),
    ]

container_pull = rule(
    impl = _container_pull_impl,
    attrs = attrs(
        reference = attr.string(mandatory = True),
    ),
    provides = [DefaultInfo, ContainerInfo],
)

def _container_impl(ctx):
    base = None
    if ctx.attrs.base:
        base = ctx.attrs.base[ContainerInfo]

    tar = ctx.attrs.tar[DefaultInfo].files

    file = ctx.actions.container.build(
        name = ctx.attrs.name,
        base = base,
        entrypoint = ctx.attrs.entrypoint,
        prioritized_files = ctx.attrs.prioritized_files,
        tar = tar,
    )
    return [
        DefaultInfo(files = [file]),
        ContainerInfo(src = file, reference = ctx.attrs.reference),
    ]

container_build = rule(
    impl = _container_impl,
    attrs = attrs(
        base = attr.label(),  # TODO: provider image
        entrypoint = attr.string_list(),
        prioritized_files = attr.string_list(),
        tar = attr.label(mandatory = True),
        reference = attr.string(mandatory = True),
    ),
    provides = [DefaultInfo, ContainerInfo],
)

def _container_push_impl(ctx):
    file = ctx.actions.container.push(
        name = ctx.attrs.name,
        image = ctx.attrs.image,
        reference = ctx.attrs.reference,
    )
    return [
        DefaultInfo(files = [file]),
        ContainerInfo(src = file, reference = ctx.attrs.reference),
    ]

container_push = rule(
    impl = _container_push_impl,
    attrs = attrs(
        image = attr.label(mandatory = True),  # TODO: providers...
        reference = attr.string(),
    ),
    provides = [DefaultInfo, ContainerInfo],
)
