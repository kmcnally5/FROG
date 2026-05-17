import "stdlib/db.lex" as db

conn, err = dbOpenWithPool("mssql", "server=localhost,1434;database=master;user id=sa;password=YourPasswordHere!", {
    "maxIdle": 5,
    "maxOpen": 20,
    "idleTimeout": 300,
    "lifetime": 3600
})
if err != null {
    println("Connection error: {err.message}")
    return
}
println("✓ Connected to SQL Server")

// Create test table
_, err = dbExec(conn, "DROP TABLE IF EXISTS test_users", [])
_, err = dbExec(conn, "CREATE TABLE test_users (id INT PRIMARY KEY, name VARCHAR(100), email VARCHAR(100), age INT)", [])
if err != null {
    println("Table creation error: {err.message}")
    dbClose(conn)
    return
}
println("✓ Created test_users table")

// Insert test data
_, err = dbExec(conn, "INSERT INTO test_users (id, name, email, age) VALUES (?, ?, ?, ?)", [1, "Alice Smith", "alice@example.com", 28])
_, err = dbExec(conn, "INSERT INTO test_users (id, name, email, age) VALUES (?, ?, ?, ?)", [2, "Bob Johnson", "bob@example.com", 35])
_, err = dbExec(conn, "INSERT INTO test_users (id, name, email, age) VALUES (?, ?, ?, ?)", [3, "Carol White", "carol@example.com", 32])
if err != null {
    println("Insert error: {err.message}")
    dbClose(conn)
    return
}
println("✓ Inserted 3 test records")

// Retrieve all rows with dbQuery (buffered)
rows, err = dbQuery(conn, "SELECT id, name, email, age FROM test_users ORDER BY id", [])
if err != null {
    println("Query error: {err.message}")
    dbClose(conn)
    return
}

println("\n--- Test Users (dbQuery) ---")
for row in rows {
    id = row["id"]
    name = row["name"]
    email = row["email"]
    age = row["age"]
    println("ID: {id} | Name: {name} | Email: {email} | Age: {age}")
}

// Retrieve rows one at a time with dbQueryStream
stream, err = dbQueryStream(conn, "SELECT id, name, email, age FROM test_users ORDER BY id", [])
if err != null {
    println("Stream error: {err.message}")
    dbClose(conn)
    return
}

println("\n--- Test Users (dbQueryStream) ---")
for row in stream {
    id = row["id"]
    name = row["name"]
    println("Streamed: {id} — {name}")
}

// Bulk insert a fresh batch
_, err = dbExec(conn, "DROP TABLE IF EXISTS bulk_test", [])
_, err = dbExec(conn, "CREATE TABLE bulk_test (id INT, name VARCHAR(100), score INT)", [])

n, err = dbBulkInsert(conn, "bulk_test", ["id", "name", "score"], [
    [1, "Alice", 95],
    [2, "Bob",   87],
    [3, "Carol", 91],
    [4, "Dave",  78],
    [5, "Eve",   99]
])
if err != null {
    println("Bulk insert error: {err.message}")
} else {
    println("\n✓ dbBulkInsert: {n} rows inserted")
}

rows, err = dbQuery(conn, "SELECT id, name, score FROM bulk_test ORDER BY score DESC", [])
if err != null { println(err.message) } else {
    println("--- Bulk results (by score) ---")
    for row in rows {
        id = row["id"]
        name = row["name"]
        score = row["score"]
        println("  {id}. {name} — {score}")
    }
}

// withTx: auto-commit on success
result, err = db.withTx(conn, fn(tx) {
    _, err = dbExec(tx, "UPDATE test_users SET age = age + 1 WHERE id = ?", [1])
    if err != null { return null, err }
    _, err = dbExec(tx, "UPDATE test_users SET age = age + 1 WHERE id = ?", [2])
    if err != null { return null, err }
    return "ages bumped", null
})
if err != null {
    println("withTx error: {err.message}")
} else {
    println("\n✓ withTx committed: {result}")
}

// withTx: auto-rollback on error
result, err = db.withTx(conn, fn(tx) {
    _, err = dbExec(tx, "UPDATE test_users SET age = 99 WHERE id = ?", [3])
    if err != null { return null, err }
    return null, error("TX_ABORT", "simulated failure — should roll back")
})
if err != null {
    println("✓ withTx rolled back as expected: {err.message}")
} else {
    println("ERROR: expected rollback but got: {result}")
}

// Confirm row 3 still has original age (rollback worked)
row, err = dbQueryOne(conn, "SELECT age FROM test_users WHERE id = ?", [3])
if err != null {
    println("check error: {err.message}")
} else {
    age = row["age"]
    println("✓ Row 3 age after rollback: {age} (should be 32)")
}

// dbExecReturning — INSERT and get back the inserted row
_, err = dbExec(conn, "DROP TABLE IF EXISTS returning_test", [])
_, err = dbExec(conn, "CREATE TABLE returning_test (id INT IDENTITY(1,1) PRIMARY KEY, name VARCHAR(100))", [])
rows, err = dbExecReturning(conn, "INSERT INTO returning_test (name) OUTPUT INSERTED.id, INSERTED.name VALUES (?)", ["Karl"])
if err != null {
    println("dbExecReturning error: {err.message}")
} else {
    id = rows[0]["id"]
    name = rows[0]["name"]
    println("\n✓ dbExecReturning: inserted id={id} name={name}")
}

// dbSetTimeout — 1ms timeout should fail on any real query
dbSetTimeout(conn, 1)
_, err = dbQuery(conn, "SELECT id, name FROM test_users ORDER BY id", [])
if err != null {
    println("✓ dbSetTimeout: query correctly timed out — {err.message}")
} else {
    println("ERROR: expected timeout but query succeeded")
}
dbSetTimeout(conn, 0)

dbClose(conn)
println("\n✓ Test completed successfully")
