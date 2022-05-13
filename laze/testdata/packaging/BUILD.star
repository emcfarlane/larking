load("rules/packaging.star", "tar")

tar(
    name = "hello.tar",
    srcs = ["hello.txt"],
)

tar(
    name = "helloc.tar.gz",
    srcs = ["file://testdata/cgo/helloc?goarch=amd64&goos=linux"],
    package_dir = "/usr/bin",
    strip_prefix = "testdata/cgo",
)
