# Tests of Starlark 'proto' extension.

load("encoding/proto.star", "proto")

def test_list(t):
    test = proto.file("testpb/star.proto")
    m = test.Message(strings = ["one", "two", "three"])

    i = 0
    for v in m.strings:
        t.equal(v, m.strings[i])
        i += 1