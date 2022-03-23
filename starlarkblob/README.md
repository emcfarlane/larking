## open

`open(url)` opens a blob bucket at the path returning a `bucket`.

| Parameter | Description |
| ------------- | ------------- |
| url | string <br /> URL of bucket. |

### bucket·attributes

`b.attributes(key)` gets the attributes of the blob.

| Parameter | Description |
| ------------- | ------------- |
| key | string <br /> Name of key. |

### bucket·write_all

`b.write_all(key, bytes, **kwargs)` writes all the bytes to the key.
If `conent_md5` is not set, `write_all` will compute the MD5 of `bytes`.

| Parameter | Description |
| ------------- | ------------- |
| key | string <br /> Name of key. |
| bytes | bytes|string <br /> Bytes of the file. |
| buffer_size | int <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| cache_control | string <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_disposition | string <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_encoding | string <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_language | string <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_type | string <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_md5 | string|bytes <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| metadata | dict <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| before_write | function <br /> [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |

### bucket·read_all

`b.read_all(key)` reads the entire blob with no options.

| Parameter | Description |
| ------------- | ------------- |
| key | string <br /> Name of key. |

### bucket·delete

`b.delete(key)` deletes the blob.

| Parameter | Description |
| ------------- | ------------- |
| key | string <br /> Name of key. |

### bucket·close

`b.close()` closes the bucket.
