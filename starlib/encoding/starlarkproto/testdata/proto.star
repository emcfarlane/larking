# Tests of Starlark 'proto' extension.
load("encoding/proto.star", "proto")

def test_protos(t):
    s = struct(body = "hello")
    t.eq(s, s)
    print(s)

    # Prefer load by import path for dynamic protobuf support
    #m = proto("starlarkproto.test.Message", body="Hello, world!")
    #test = proto.package("starlarkproto.test")
    #test = 1
    test = proto.file("testpb/star.proto")
    m = test.Message(body = "Hello, world!")
    t.eq(m, m)
    t.eq(dir(m), ["body", "maps", "nested", "one_number", "one_string", "oneofs", "strings", "type"])
    print(m)
    t.eq(m.body, "Hello, world!")

    # Setting value asserts types
    def set_field_invalid():
        m.body = 2

    t.fails(set_field_invalid, "proto: *")

    # Enums
    enum = proto.new("starlarkproto.test.Enum")
    enum_a = enum(0)
    enum_a_alt = enum("ENUM_A")
    t.eq(enum_a, enum_a_alt)

    enum_file = test.Enum
    enum_b = enum_file(1)
    enum_b_alt = enum_file("ENUM_B")
    t.eq(enum_b, enum_b_alt)
    t.ne(enum_a, enum_b)
    #print("ENUMS", enum_a, enum_b)

    # Nested Enums
    message_unknown = test.Message.Type.UNKNOWN
    message_greeting = test.Message.Type.GREETING
    t.ne(message_unknown, message_greeting)

    # Enums can be assigned by String or Ints
    t.eq(m.type, message_unknown)
    m.type = "GREETING"
    t.eq(m.type, message_greeting)
    m.type = 0
    t.eq(m.type, message_unknown)
    m.type = message_greeting
    t.eq(m.type, message_greeting)

    # Lists are references
    b = m.strings
    b.append("hello")
    t.eq(m.strings[0], "hello")

    #print(m)
    b.extend(["world", "it", "is", "me"])
    t.eq(len(m.strings), 5)
    slice = m.strings[0:5:2]
    t.eq(slice, ["hello", "it", "me"])
    t.eq(len(m.strings), 5)
    m.strings = slice
    t.eq(len(m.strings), 3)

    # Message can be created from structs
    m.nested = struct(body = "struct", type = "GREETING")
    t.eq(m.nested.body, "struct")
    t.eq(m.nested.type, message_greeting)
    print(m)

    # Messages can be assigned None to delete
    m.nested = None

    #t.eq(m.nested, test.Message(None))  # None creates typed nil
    t.true(not m.nested, msg = "Nil RO type is falsy")  #
    t.eq(m.nested.nested.body, "")  # Recursive nil returns default types

    # Messages can be created from maps
    m.nested = {"body": "map"}
    t.eq(m.nested.body, "map")
    mmap = test.Message({"body": "new map"})
    t.eq(mmap.body, "new map")

    # Messages can be assigned Messages
    nested = test.Message(body = "nested")
    m.nested = nested

    # Maps shallow copy Dicts on set
    m.maps = {
        "hello": struct(body = "world!", type = "GREETING"),
    }
    print(m)

    # Oneofs
    m.one_string = "one dream"
    t.eq(m.one_string, "one dream")
    t.eq(m.one_number, 0)
    t.eq(m.oneofs, "one dream")
    m.one_number = 1
    t.eq(m.one_string, "")
    t.eq(m.one_number, 1)
    t.eq(m.oneofs, 1)
    #print(m)

    # Marshal/Unmarshal
    data = proto.marshal(m)
    m2 = test.Message()
    proto.unmarshal(data, m2)
    t.eq(m, m2)

    # print(proto.marshal_json(m))
    # print(proto.marshal_text(m))

##def test_load(t):
##    proto.load(library_bin)
##
##    apipb = proto.file("larking.examples.apipb")
##    book = apipb.Book(
##        name = req.name,
##        title = "A book appears!",
##        author = "starlark",
##    )
##    print("created book: %s" % book)
