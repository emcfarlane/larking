# Tests of Starlark 'proto' extension.

load("assert.star", "assert")

b = blob.open("mem://")
b.write_all("note.txt", "hello")
wrote = b.read_all("note.txt")
assert.eq(wrote, b"hello")
