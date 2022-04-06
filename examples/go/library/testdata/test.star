svc = mux.service("larking.examples.go.library.api")
print(svc)
print(dir(svc))

def test_get_book(t):
    print(dir(mux))
    print(dir(mux.service("larking.examples.go.library.api")))
    req = svc.GetBook(name = "shelves/1/books/1")
