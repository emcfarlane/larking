# Tests of Starlark 'grpc' extension.

# TODO: show dialing to add a new stream.
#grpc.dial("//")

s = mux.service("larking.testpb.Messaging")

def test_message(t):
    print(s)
    m = s.GetMessageOne({
        "name": "starlark",
    })

    t.eq(m.message_id, "starlark")
    t.eq(m.text, "hello")
    t.eq(m.user_id, "user")

# TODO: handle the reflection stream.
#pb = proto.new("larking.testpb")
#x = pb.Message(message_id = "starlark", text = "hello", user_id = "user")
