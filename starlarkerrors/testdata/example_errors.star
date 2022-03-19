load("errors.star", "errors")
load("assert.star", "assert")

def assert_even(x):
    if x % 2 == 0:
        fail("odd")
    return x

# catch the error and check the result.
# Results will be truthy if valid, and falsy if not.
# Okay values can be accessed by the .val attribute.
res = errors.catch(assert_even, 2)
if res:
    print("% is even!", res.val)

# Error values can be accessed by the .err attribute.
res = errors.catch(assert_even, 3)
if not res:
    print("error: %", res.err)
