load("std.star", "errors")


def assert_even(x):
    if x % 2 != 0:
        fail("odd")
    return x


def test_catch(t):
    # catch the error and check the result.
    # Results will be truthy if valid, and falsy if not.
    # Okay values can be accessed by the .val attribute.
    res = errors.catch(assert_even, 2)
    t.eq(res.val, 2)

    # Error values can be accessed by the .err attribute.
    res = errors.catch(assert_even, 3)
    t.true(res.err.matches("odd"))


def test_sequence_assigment(t):
    # try the function, and return the result if it succeeds.
    # If it fails, return the default value.
    res, err = errors.catch(assert_even, 2)
    t.eq(err, None)
    print("% is even!", res)

    # If the default value is not provided, None will be returned.
    res, err = errors.catch(assert_even, 3)
    print("error: %", err)
    print("res: %", res)
    t.eq(res, None)
    err.matches("odd")
