load("rules/container.star", "container_build", "container_pull", "container_push")

# base image
container_pull(
    name = "distroless.tar",
    reference = "gcr.io/distroless/base:nonroot",
)

# helloc is an image based on cross compiling packaging
container_build(
    name = "helloc.tar",
    base = "distroless.tar",
    entrypoint = ["/usr/bin/helloc"],
    #labels = ["latest"],
    prioritized_files = ["/usr/bin/hello"],  # Supports estargz.
    reference = "gcr.io/star-c25e4/helloc:latest",
    tar = "../packaging/helloc.tar.gz",
)

# push the image to a registry
container_push(
    name = "myrepo",
    image = "helloc.tar",
    reference = "gcr.io/star-c25e4/helloc:latest",
)
