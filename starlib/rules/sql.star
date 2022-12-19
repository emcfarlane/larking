load("rule.star", "rule", "attrs", "attr", "label", "provides")
load("sql.star", "sql")

# create
def create(db):
    db.exec("CREATE TABLE projects(mascot VARCHAR(10), release SMALLINT, category TEXT NOT NULL)")

# insert
def insert(db):
    projects = [
        ("tux", 1991),
        ("duke", 1996),
        ("gopher", 2009),
        ("moby dock", 2013),
    ]

    stmt = "INSERT INTO projects(mascot, release, category) VALUES( ?, ?, ? )"
    for project in projects:
        db.exec(stmt, project[0], project[1], "open source")


def _query_impl(name, database, statement, args=[]):
    """Test rule takes name as input and returns a string output."""
    print(database, statement, args)

    db = sql.open("sqlite:file::memory:?cache=shared")

    
    create(db)
    print("created")
    insert(db)
    print("insert")
    
    rows = db.query(statement, *args)
    rows = [row for row in rows]  # copy values out of rows loop
    print("rows", rows)

    return [rows]


query = rule(
    impl = _query_impl,
    attrs = attrs(
        database = attr.string(),
        statement = attr.string(),
        args = attr.list(val_kind = "any", optional=True),
    ),
    provides = provides(
        attr.string(),
    ),
)
