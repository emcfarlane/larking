# Tests of Starlark 'nethttp' extension.

def test_get(t):
    rsp = http.get(addr + "/hello")
    print(rsp)
    t.eq(rsp.body.read_all(), b"world\n")
