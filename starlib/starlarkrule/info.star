# Default providers

DefaultInfo = attrs(
    files = attr.label_list(
        doc = "A list of files.",
        mandatory = True,
    ),
    executable = attr.label(
        doc = "Executable file, if runnable.",
        mandatory = False,
    ),
)

ContainerInfo = attrs(
    src = attr.label(
        doc = "Tarball source of container.",
        mandatory = True,
    ),
    reference = attr.string(
        doc = "Reference.",
        mandatory = True,
    ),
)
