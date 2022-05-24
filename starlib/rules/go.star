load("rule.star", "DefaultInfo", "attr", "provider", "rule")
load("thread.star", "arch", "os")

# TODO: filepath.Join() support for pathing...
def _go_impl(ctx):
    print("GOT CTX", ctx)
    args = ["build", "-o", ctx.attrs.name]

    env = []
    goos = os or ctx.attrs.goos
    env.append("GOOS=" + goos)

    goarch = arch or ctx.attrs.goarch
    env.append("GOARCH=" + ctx.attrs.goarch)

    if ctx.attrs.cgo:
        # TODO: better.
        target = {"amd64": "x86_64"}[goarch] + "-" + goos

        env.append("CGO_ENABLED=1")
        env.append("CC=zig cc -target x86_64-linux")  # + ctx.attrs._zcc.value.path)
        env.append("CXX=zig c++ -target x86_64-linux")  #  + ctx.attrs._zxx.value.path)
    else:
        env.append("CGO_ENABLED=0")
    print("ENV:", env)

    args.append(".")

    # Maybe?
    ctx.actions.run(
        name = "go",
        dir = ctx.build_dir,
        args = args,
        env = env,
    )

    name = ctx.actions.label(ctx.attrs.name)
    print("name", name)
    return ctx.outs(
        bin = name,
    )

go = rule(
    impl = _go_impl,
    #attrs = provider(
    #    goos = attr.string(values = [
    #        ,
    #    ])
    #)
    #provides = [DefaultInfo]
    ins = {
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
    outs = {
        "bin": attr.label(
            executable = True,
            mandatory = True,
        ),
    },
)
