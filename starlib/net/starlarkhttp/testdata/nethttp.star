# Tests of Starlark 'nethttp' extension.
load("net/http.star", "http")

def test_get(t):
    rsp = http.get(addr + "/hello")
    print(rsp)
    t.eq(rsp.body.read_all(), b"world\n")
