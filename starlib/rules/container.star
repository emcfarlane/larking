load("rule.star", "attr", "rule")

def _container_pull_impl(ctx):
    return ctx.actions.container.pull(
        name = ctx.attrs.name,
        reference = ctx.attrs.reference,
    )

container_pull = rule(
    impl = _container_pull_impl,
    attrs = {
        "reference": attr.string(mandatory = True),
    },
)

def _container_impl(ctx):
    return ctx.actions.container.build(
        name = "",
        base = ctx.attrs.base,
        entrypoint = ctx.attrs.entrypoint,
        prioritized_files = ctx.attrs.prioritized_files,
        tar = ctx.attrs.tar,
    )

container_build = rule(
    impl = _container_impl,
    attrs = {
        "base": attr.label(),  # TODO: provider image
        "entrypoint": attr.string_list(),
        #"labels": attr.string_list(),  # TODO
        "prioritized_files": attr.string_list(),
        "tar": attr.label(),
    },
)

def _container_push_impl(ctx):
    return ctx.actions.container.push(
        name = ctx.attrs.name,
        image = ctx.attrs.image,
        reference = ctx.attrs.reference,
    )

container_push = rule(
    impl = _container_push_impl,
    attrs = {
        "image": attr.label(mandatory = True),  # TODO: providers...
        "reference": attr.string(),
    },
)
