// stdlib/db.lex — Database transaction helpers
//
// Provides higher-level patterns built on the dbBegin/dbCommit/dbRollback builtins.
//
// Usage:
//   import "db.lex" as db
//
//   result, err = db.withTx(conn, fn(tx) {
//       _, err = dbExec(tx, "INSERT INTO audit (user_id) VALUES (?)", [42])
//       if err != null { return null, err }
//       _, err = dbExec(tx, "UPDATE accounts SET balance = balance - ? WHERE id = ?", [100, 42])
//       if err != null { return null, err }
//       return "transfer complete", null
//   })
//   if err != null {
//       println("Transaction failed: {err.message}")
//   } else {
//       println(result)
//   }

// withTx(conn, body) → (result, err)
//
// Runs body(tx) inside a transaction.
// Commits automatically if body returns (value, null).
// Rolls back automatically if body returns (value, err) with err != null.
// Rolls back automatically if dbCommit itself fails.
//
// body must return a (value, err) tuple — the standard kLex two-path pattern.
// Any error returned from body is passed through as the error of withTx.
fn withTx(conn, body) {
    tx, beginErr = dbBegin(conn)
    if beginErr != null { return null, beginErr }

    result, bodyErr = body(tx)
    if bodyErr != null {
        dbRollback(tx)
        return null, bodyErr
    }

    _, commitErr = dbCommit(tx)
    if commitErr != null {
        dbRollback(tx)
        return null, commitErr
    }

    return result, null
}
