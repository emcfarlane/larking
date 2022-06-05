load("blob.star", "read_all")
load("encoding/proto.star", "proto")

library_bin = read_all("apipb/library.bin")
print("loading library.bin")

apipb = proto.file("apipb/library.proto")

def get_book(req):
    print(req)
    return apipb.Book(
        name = req.name,
        title = "A book appears!",
        author = "starlark",
    )

mux.register_service(
    "larking.examples.apipb.Library",
    GetBook = get_book,
)
