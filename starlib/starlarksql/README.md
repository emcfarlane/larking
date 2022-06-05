## open

`open(url)` opens a SQL database returning a new `db`.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| url | string | URL of SQL database. |

### db·exec

`b.exec(query, *args)` executes a query that doesn't return rows, like INSERT or 
UPDATE.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| query | string | SQL query. |
| *args | any | Query arguments. |

### db·query

`d.query(query, *args)` executes a query that returns rows, typically a SELEECT.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| query | string | SQL query. |
| *args | any | Query arguments. |

### db·query_row

`d.query_row(query, *args)` executes a query that is expected to return at most one row.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| query | string | SQL query. |
| *args | any | Query arguments. |

### db·ping

`d.ping()` the database, good for checking the connection status.

### db·close

`d.close()` closes the database connection.

## errors

- `err_conn_done`
- `err_no_rows`
- `err_tx_done`
