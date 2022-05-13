load("rules/go.star", "go")

print("go", go)

go(
    name = "helloc",
    cgo = True,
)
