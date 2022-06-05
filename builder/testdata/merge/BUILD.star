load("concat.star", "concat")
load("thread.star", "thread")

concat(
    name = "text.txt",
    chunks = [
        "into.txt",
        "body.txt",
    ],
    merge_tool = {
        "linux": "merge.sh",
        "windows": "merge.bat",
    }.get(
        thread.os,
        "merge.sh",
    ),
)
