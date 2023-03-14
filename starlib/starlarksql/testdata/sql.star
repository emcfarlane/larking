# Tests of Starlark 'sql' extension.
load("sql.star", "sql")

def test_sql(assert):
    # db resource is shared, safe for concurrent access.
    db = sql.open("sqlite:file::memory:?cache=shared")
    assert.true(db)  # check database active.

    db.ping()

    # create
    def create():
        db.exec("CREATE TABLE projects(mascot VARCHAR(10), release SMALLINT, category TEXT NOT NULL)")

    # insert
    def insert():
        projects = [
            ("tux", 1991),
            ("duke", 1996),
            ("gopher", 2009),
            ("moby dock", 2013),
        ]

        stmt = "INSERT INTO projects(mascot, release, category) VALUES( ?, ?, ? )"
        for project in projects:
            db.exec(stmt, project[0], project[1], "open source")

    # query
    def query(after):
        rows = db.query("SELECT rowid, mascot, release, category FROM projects WHERE release > ? ORDER BY release ASC", after)
        return [row for row in rows]  # copy values out of the iterator

    create()
    insert()

    query_rows = query(2008)
    assert.eq(len(query_rows), 2)
    print("projects:", query_rows)

    sel_row = query_rows[0]
    one_row = db.query_row("SELECT * FROM projects WHERE mascot = ?", sel_row[1])
    print("row:", one_row)

    if assert.eq(len(one_row), 3):
        assert.eq("gopher", sel_row[1])
        assert.eq(2009, sel_row[2])
        assert.eq("open source", sel_row[3])
