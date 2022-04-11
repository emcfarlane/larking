## open

`open(url)` opens a blob bucket at the path returning a `bucket`.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| url | string | URL of bucket. |

### bucket·attributes

`b.attributes(key)` gets the attributes of the blob.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| key | string | Name of key. |

### bucket·write_all

`b.write_all(key, bytes, **kwargs)` writes all the bytes to the key.
If `conent_md5` is not set, `write_all` will compute the MD5 of `bytes`.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| key | string | Name of key. |
| bytes | bytes|string | Bytes of the file. |
| buffer_size | int | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| cache_control | string | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_disposition | string | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_encoding | string | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_language | string | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_type | string | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| content_md5 | string, bytes | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| metadata | dict | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |
| before_write | function | [*blob.WriterOptions](https://pkg.go.dev/gocloud.dev/blob#WriterOptions). |

### bucket·read_all

`b.read_all(key)` reads the entire blob with no options.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| key | string | Name of key. |

### bucket·delete

`b.delete(key)` deletes the blob.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| key | string | Name of key. |

### bucket·close

`b.close()` closes the bucket.
