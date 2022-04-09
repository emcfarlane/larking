load("proto.star", "proto")

# Load the proto file descriptor to create.
apipb = proto.file("apipb/library.proto")

def test_get_book(t):
    svc = mux.service("larking.examples.library.Library")

    book = apipb.Book(
        title = "Larking Guide",
        author = "Edward",
    )

    rsp1 = svc.CreateBook(
        parent = "shelves/guides",
        book = book,
    )
    print("created book %s" % rsp1.name)

    # Check the book was created correctly
    t.eq(rsp1.title, book.title)
    t.eq(rsp1.author, book.author)

    rsp2 = svc.GetBook(
        name = rsp1.name,
    )
    print("got book %s" % rsp2.name)

    # Assert API respones are equal
    t.eq(rsp1, rsp2)
