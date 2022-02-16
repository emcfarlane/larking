# what
load("assert.star", "assert")

# A comment?
b = blob.open("mem://")
b.write_all("note.txt", "hello")  # a comment
wrote = b.read_all("note.txt")
assert.eq(wrote, b"hello")
