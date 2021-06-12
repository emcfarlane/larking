# Tests of Starlark 'grpc' extension.

load("assert.star", "assert")

# TODO: show dialing to add a new stream.
#grpc.Dial("//")

s = grpc.service("larking.testpb.Messaging")
m = s.GetMessageOne({
    "name": "starlark",
})

assert.eq(m.message_id, "starlark")
assert.eq(m.text, "hello")
assert.eq(m.user_id, "user")

# TODO: handle the reflection stream.
#pb = proto.new("larking.testpb")
#x = pb.Message(message_id = "starlark", text = "hello", user_id = "user")
#assert.eq(m, x)
