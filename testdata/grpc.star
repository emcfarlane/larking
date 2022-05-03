# Tests of Starlark 'grpc' extension.

load("proto.star", "proto")

# TODO: show dialing to add a new stream.
#grpc.dial("//")

def test_message(t):
    s = mux.service("larking.testpb.Messaging")

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

pb = proto.file("testpb/test.proto")
bodypb = proto.file("google/api/httpbody.proto")

# Register a starlark service.
def test_register_service(t):
    t.skip()  # TODO: ensure service is called.
    s = mux.service("larking.testpb.Files")

    def upload_download(req):
        print("TODO: capture me")
        return bodypb.HttpBody(
            content_type = "text/plain",
            data = str(req.file.data) + ", starlark!",
        )

    def large_upload_download(stream):
        req = stream.recv()
        print("upload request", req)

        msg = pb.Mesasge(
            message_id = "123",
            text = "hello, starlark!",
        )
        data = proto.marshal_json(msg)
        n = len(data) / 3
        chunks = [data[i:i + n] for i in range(0, len(data), n)]

        for chunk in chunks:
            stream.send(bodypb.HttpBody(
                content_type = "application/json",
                data = chunk,
            ))

    # register_service
    mux.register_service(
        "larking.testpb.Files",
        UploadDownload = upload_download,
        LargeUploadDownload = large_upload_download,
    )

    rsp = s.UploadDownload(
        filename = "message.txt",
        file = bodypb.HttpBody(
            content_type = "text/plain",
            data = "hello",
        ),
    )
    print(rsp)
    t.eq(rsp.content_type, "text/plain")
    t.eq(rsp.data, "hello, starlark!")

    # TODO: streaming...
    #stream = s.LargeUploadDownload()
    #stream.send()
    #stream.recv()
