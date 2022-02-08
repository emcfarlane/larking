# Tests of Starlark 'nethttp' extension.

load("assert.star", "assert")

rsp = http.get(addr + "/hello")
print(rsp)
assert.eq(rsp.body.read_all(), b"world\n")
