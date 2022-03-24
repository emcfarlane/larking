## open

`open(url)` opens a SQL database returning a new `db`.

| Parameter | Description |
| ------------- | ------------- |
| url | string <br /> URL of SQL database. |

### db·exec

`b.exec(query, *args)` executes a query that doesn't return rows, like INSERT or 
UPDATE.

| Parameter | Description |
| ------------- | ------------- |
| query | string <br /> SQL query. |
| *args | any <br /> Query arguments. |

### db·query

`d.query(query, *args)` executes a query that returns rows, typically a SELEECT.

| Parameter | Description |
| ------------- | ------------- |
| query | string <br /> SQL query. |
| *args | any <br /> Query arguments. |

### db·query_row

`d.query_row(query, *args)` executes a query that is expected to return at most one row.

| Parameter | Description |
| ------------- | ------------- |
| query | string <br /> SQL query. |
| *args | any <br /> Query arguments. |

### db·ping

`d.ping()` the database, good for checking the connection status.

### db·close

`d.close()` closes the database connection.

## errors

- `err_conn_done`
- `err_no_rows`
- `err_tx_done`
