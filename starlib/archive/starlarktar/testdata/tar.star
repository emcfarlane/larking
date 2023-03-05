
# encode 

load("")

def test_example(t):
    buf = bytes.new_buffer()
    tw = tar.new_writer(buf)

    objs = [
        ("readme.txt", "This archive contains some text files."),
        ("gopher.txt", "Gopher names:\nGeorge\nGeoffrey\nGonzo"),
        ("todo.txt", "Get animal handling license."),
    ]

    for obj in objs:
        tw.write_header(
            name = obj[0],
            mode = 0600,
            size = len(obj[1])),
        )
        tw.write(obj[1])

    tw.close()

    tr = tar.new_reader(buf) 

    for file in tr:
        print("reading file %s" % os.name)
        io.copy(os.std_out, file)
        print()
