load("blob.star", "blob")

def test_write_all(t):
    b = blob.open("mem://")
    b.write_all("note.txt", "hello")  # a comment
    wrote = b.read_all("note.txt")
    t.eq(wrote, b"hello")
