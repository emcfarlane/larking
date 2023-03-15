# Tests of Starlark 'proto' extension.
load("std.star", proto = "encoding/proto")

def test_message_str(t):
    test = proto.file("testpb/star.proto")

    m = test.Message(
        body = "Hello, world",
        nested = test.Message(
            body = "nested",
        ),
    )
    t.equal(str(m), "Message(body = \"Hello, world\", type = UNKNOWN, strings = [], nested = Message(body = \"nested\", type = UNKNOWN, strings = [], nested = Message(None), maps = {}, one_string = \"\", one_number = 0), maps = {}, one_string = \"\", one_number = 0)")
