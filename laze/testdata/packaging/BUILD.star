load("rules/packaging.star", "tar")

tar(
    name = "helloc.tar.gz",
    srcs = ["file://testdata/cgo/helloc?goarch=amd64&goos=linux"],
    package_dir = "/usr/bin",
    strip_prefix = "testdata/cgo",
)
