# Tests of Starlark 'errors' extension.
load("errors.star", "errors")

err_msg = "hello"
err_val = errors.error("custom error [%s]" % (err_msg))
print(err_val)

def failing_func():
    fail("something went wrong")

r1 = errors.catch(failing_func)

def test_catch(t):
    t.true(not r1)
    t.true(r1.err)  # error is truthy
    t.true(r1.err.matches(".* wrong"))

def access_value():
    return r1.val

# trying to access the value fails with the original error
def test_fails(t):
    t.fails(access_value, "something went wrong")

def hello(name):
    return "hello, " + name

def test_errors(t):
    r2 = errors.catch(hello, "world")
    t.true(r2)
    t.true(not r2.err)

    t.eq(r2.val, "hello, world")

    r3 = errors.catch(io_eof_func)
    t.true(r3.err)

    # check error is type of io.EOF error
    t.true(r3.err.kind(io_eof))

    # TODO: handle extracting error values
    #path_err = r4.err.as(io_path_err)
    #if path_err:
    #    print("got path error")
