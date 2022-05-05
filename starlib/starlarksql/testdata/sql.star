# Tests of Starlark 'sql' extension.
load("sql.star", "sql")

def test_sql(t):
    # db resource is shared, safe for concurrent access.
    db = sql.open("sqlite:file::memory:?cache=shared")
    t.true(db)  # check database active.

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
        rows = db.query("SELECT rowid, * FROM projects WHERE release > ? ORDER BY release ASC", after)
        return [row for row in rows]  # copy values out of rows loop

    create()
    insert()

    query_rows = query(2008)
    t.eq(len(query_rows), 2)
    print("projects:", query_rows)

    sel_row = query_rows[0]
    one_row = db.query_row("SELECT * FROM projects WHERE mascot = ?", sel_row.mascot)
    print("row:", one_row)

    t.eq(one_row.mascot, sel_row.mascot)
    t.eq(one_row.release, sel_row.release)
    t.eq(one_row.category, sel_row.category)
