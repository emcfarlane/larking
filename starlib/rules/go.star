load("rule.star", "attr", "rule", "actions", "label")

# TODO: filepath.Join() support for pathing...
def _go_impl(ctx):
    args = ["build", "-o", ctx.attrs.name]

    env = []
    if ctx.attrs.goos != "":
    	env.append("GOOS=" + ctx.attrs.goos)

    if ctx.attrs.goarch != "":
    	env.append("GOARCH=" + ctx.attrs.goarch)

    if ctx.attrs.cgo:
        env.append("CGO_ENABLED=1")
        env.append("CC=" + ctx.attrs._zcc.value.path)
        print("ZCC", ctx.attrs._zcc)
        env.append("CXX=" + ctx.attrs._zxx.value.path)
    else:
        env.append("CGO_ENABLED=0")
    print("ENV:", env)

    args.append(".")

    # Maybe?
    ctx.actions.run(
        name = "go",
        args = args,
        env = env,
    )
    #name = actions.path.join(ctx.build_dir, ctx.attrs.name)
    name = label(ctx.build_dir, ctx.attrs.name)
    print("name", name)
    return struct(
        executable = name,
    )
    #ctx.actions.files.stat(
    #    name = ctx.build_dir + "/" + ctx.attrs.name,
    #)

go = rule(
    impl = _go_impl,
    attrs = {
        "goos": attr.string(values = [
            "aix",
            "android",
            "darwin",
            "dragonfly",
            "freebsd",
            "hurd",
            "illumos",
            "js",
            "linux",
            "nacl",
            "netbsd",
            "openbsd",
            "plan9",
            "solaris",
            "windows",
            "zos",
        ]),
        "goarch": attr.string(values = [
            "386",
            "amd64",
            "amd64p32",
            "arm",
            "armbe",
            "arm64",
            "arm64be",
            "ppc64",
            "ppc64le",
            "mips",
            "mipsle",
            "mips64",
            "mips64le",
            "mips64p32",
            "mips64p32le",
            "ppc",
            "riscv",
            "riscv64",
            "s390",
            "s390x",
            "sparc",
            "sparc64",
            "wasm",
        ]),
        "cgo": attr.bool(),
        #"_zxx": attr.label(allow_files = True, default = "file://rules/go/zxx"),
        #"_zcc": attr.label(allow_files = True, default = "file://rules/go/zcc"),
    },
    returns = {
        "executable": attr.label(
            executable = True,
            allow_single_file = [],
            mandatory = True,
            #providers = [],
        ),
    },
)
