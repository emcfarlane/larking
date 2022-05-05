# Tests of Starlark 'runtimevar' extension.
load("runtimevar.star", "runtimevar")
load("time.star", "time")

def test_runtimevar(t):
    # variable resource is shared, safe for concurrent access.
    val = runtimevar.open("constant://?val=hello+world&decoder=string")
    print(val)

    t.eq(val.latest().value, "hello world")
    t.true(val)
    t.lt(val.latest().update_time, time.now())
