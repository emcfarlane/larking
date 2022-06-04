load("rule.star", "DefaultInfo")

def test_default_attrs(t):
    info = DefaultInfo(
        files = ["source"],
    )

    print(info)
    t.eq(len(info.files), 1)
    file = info.files[0]
    t.eq(file.key, "source")
