load("rules/go.star", "go")

go(
    name = "helloc",
    cgo = True,
)
