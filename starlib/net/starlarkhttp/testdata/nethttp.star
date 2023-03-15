# Tests of Starlark 'nethttp' extension.
load("std.star", http = "net/http")

def test_get(t):
    rsp = http.get(addr + "/hello")
    print(rsp)
    t.eq(rsp.body.read_all(), b"world\n")
