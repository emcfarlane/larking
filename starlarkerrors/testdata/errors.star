# Tests of Starlark 'errors' extension.

load("assert.star", "assert")

err_msg = "hello"
err_val = errors.new("custom error [%s]" % (err_msg))
print(err_val)

def failing_func():
    fail("something went wrong")

r1 = errors.call(failing_func)

assert.true(not r1)
assert.true(r1.err)  # error is truthy
assert.true(r1.err.matches(".* wrong"))

def access_value():
    return r1.val

# trying to access the value fails with the original error
assert.fails(access_value, "something went wrong")

def hello(name):
    return "hello, " + name

r2 = errors.call(hello, "world")
assert.true(r2)
assert.true(not r2.err)

assert.eq(r2.val, "hello, world")

r3 = errors.call(io_eof_func)
assert.true(r3.err)

# check error is type of io.EOF error
assert.true(r3.err.kind(io_eof))

# TODO: handle extracting error values
#path_err = r4.err.as(io_path_err)
#if path_err:
#    print("got path error")
