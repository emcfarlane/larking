# Tests of Starlark 'runtimevar' extension.

load("assert.star", "assert")

# variable resource is shared, safe for concurrent access.
val = runtimevar.open("constant://?val=hello+world&decoder=string")
print(val)
assert.true(val)

assert.eq(val.latest().value, "hello world")
assert.lt(val.latest().update_time, time.now())
