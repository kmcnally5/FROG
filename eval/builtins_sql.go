package eval

import (
	"context"
	"database/sql"
	"fmt"
	"klex/ast"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"       // registers "pgx" driver
	_ "github.com/microsoft/go-mssqldb"       // registers "mssql" and "sqlserver" drivers
)

// sqlError builds a user-visible (non-propagating) error tuple: (null, error).
func sqlError(code, msg string) Object {
	return &Tuple{Elements: []Object{
		NULL,
		&Error{IsUserError: true, Code: code, Message: msg},
	}}
}

// sqlOk builds a success tuple: (value, null).
func sqlOk(v Object) Object {
	return &Tuple{Elements: []Object{v, NULL}}
}

// sqlResolveDriver maps user-friendly driver names to the registered Go driver name.
func sqlResolveDriver(name string) string {
	if name == "postgres" {
		return "pgx"
	}
	return name // "mssql", "sqlserver", "pgx" passed through as-is
}

// sqlMakeContext returns a context honouring the timeout stored on conn or tx.
// Returns context.Background() (no deadline) when Timeout is zero.
// Caller must call cancel() when done — safe to defer even for Background.
func sqlMakeContext(arg Object) (context.Context, context.CancelFunc) {
	var timeout time.Duration
	switch v := arg.(type) {
	case *DBConn:
		timeout = v.Timeout
	case *DBTx:
		timeout = v.Timeout
	}
	if timeout > 0 {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.Background(), func() {}
}

// sqlQueryable extracts a common query interface from either a DBConn or DBTx.
// Both *sql.DB and *sql.Tx implement QueryContext and ExecContext.
type sqlQueryable interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func sqlExtract(arg Object, fnName string) (sqlQueryable, string, *Error) {
	switch v := arg.(type) {
	case *DBConn:
		return v.DB, v.Driver, nil
	case *DBTx:
		return v.Tx, v.Driver, nil
	default:
		return nil, "", typeError(
			fmt.Sprintf("%s: first argument must be a db connection or transaction, got %s", fnName, arg.Type()),
			ast.Pos{},
		)
	}
}

// sqlBuildArgs converts an optional kLex Array of query parameters to []interface{}.
func sqlBuildArgs(args []Object, offset int, fnName string) ([]interface{}, *Error) {
	if len(args) <= offset {
		return nil, nil
	}
	arr, ok := args[offset].(*Array)
	if !ok {
		return nil, typeError(
			fmt.Sprintf("%s: args must be an array, got %s", fnName, args[offset].Type()),
			ast.Pos{},
		)
	}
	out := make([]interface{}, len(arr.Elements))
	for i, el := range arr.Elements {
		out[i] = kLexToSQL(el)
	}
	return out, nil
}

// kLexToSQL converts a kLex Object to a Go value suitable for database/sql.
func kLexToSQL(v Object) interface{} {
	switch val := v.(type) {
	case *Integer:
		return int64(val.Value)
	case *Float:
		return val.Value
	case *Boolean:
		return val.Value
	case *String:
		return val.Value
	case *Null:
		return nil
	default:
		return val.Inspect()
	}
}

// sqlToKLex converts a scanned SQL value (interface{}) to a kLex Object.
func sqlToKLex(v interface{}) Object {
	if v == nil {
		return NULL
	}
	switch val := v.(type) {
	case int64:
		return &Integer{Value: int(val)}
	case float64:
		return &Float{Value: val}
	case bool:
		if val {
			return TRUE
		}
		return FALSE
	case string:
		return &String{Value: val}
	case []byte:
		return &String{Value: string(val)}
	case time.Time:
		return &String{Value: val.Format(time.RFC3339)}
	default:
		return &String{Value: fmt.Sprintf("%v", val)}
	}
}

// sqlRowsToArray converts a *sql.Rows result set to a kLex Array of Hashes.
// Each hash has column names as string keys and scanned values as kLex objects.
func sqlRowsToArray(rows *sql.Rows) (Object, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []Object
	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		h := &Hash{Pairs: make(map[HashKey]HashPair, len(cols))}
		for i, col := range cols {
			key := &String{Value: col}
			hk := HashKey{Type: STRING_OBJ, Value: col}
			h.Pairs[hk] = HashPair{Key: key, Value: sqlToKLex(raw[i])}
		}
		result = append(result, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	elements := make([]Object, len(result))
	copy(elements, result)
	return &Array{Elements: elements}, nil
}

func init() {
	// dbOpen(driver, dsn) → (conn, err)
	//
	// Opens a database connection pool and verifies connectivity with a ping.
	// The pool is safe for concurrent use — share one conn across goroutines.
	//
	// Supported drivers:
	//   "mssql"      — Microsoft SQL Server (also accepts "sqlserver")
	//   "postgres"   — PostgreSQL via pgx
	//
	// Connection strings:
	//   MS SQL:   "server=host;database=mydb;user id=sa;password=Pass1!"
	//             "sqlserver://sa:Pass1!@host:1433?database=mydb"
	//   Postgres: "host=host user=user password=pass dbname=mydb sslmode=disable"
	//             "postgres://user:pass@host:5432/mydb"
	//
	// Example:
	//   conn, err = dbOpen("mssql", "server=myserver;database=Sales;user id=sa;password=Pass1!")
	//   if err != null { println(err.message)  return }
	Builtins["dbOpen"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("dbOpen expects 2 arguments (driver, dsn)", ast.Pos{})
		}
		driverArg, ok1 := args[0].(*String)
		dsnArg, ok2 := args[1].(*String)
		if !ok1 {
			return typeError(fmt.Sprintf("dbOpen: driver must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if !ok2 {
			return typeError(fmt.Sprintf("dbOpen: dsn must be string, got %s", args[1].Type()), ast.Pos{})
		}
		goDriver := sqlResolveDriver(driverArg.Value)
		db, err := sql.Open(goDriver, dsnArg.Value)
		if err != nil {
			return sqlError("DB_OPEN_ERROR", err.Error())
		}
		if err := db.PingContext(context.Background()); err != nil {
			db.Close()
			return sqlError("DB_CONNECT_ERROR", err.Error())
		}
		return sqlOk(&DBConn{DB: db, Driver: driverArg.Value})
	}}

	// dbClose(conn) → null
	//
	// Closes the connection pool. Best effort — call when done.
	Builtins["dbClose"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("dbClose expects 1 argument (conn)", ast.Pos{})
		}
		conn, ok := args[0].(*DBConn)
		if !ok {
			return typeError(fmt.Sprintf("dbClose: argument must be a db connection, got %s", args[0].Type()), ast.Pos{})
		}
		conn.DB.Close()
		return NULL
	}}

	// dbPing(conn) → (null, err)
	//
	// Verifies the connection is still alive. Useful for health checks.
	Builtins["dbPing"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("dbPing expects 1 argument (conn)", ast.Pos{})
		}
		conn, ok := args[0].(*DBConn)
		if !ok {
			return typeError(fmt.Sprintf("dbPing: argument must be a db connection, got %s", args[0].Type()), ast.Pos{})
		}
		if err := conn.DB.PingContext(context.Background()); err != nil {
			return sqlError("DB_PING_ERROR", err.Error())
		}
		return sqlOk(NULL)
	}}

	// dbQuery(conn, sql, ?args) → (rows, err)
	//
	// Executes a SELECT and returns all rows as an array of hashes.
	// Column names become hash keys. SQL NULLs become kLex null.
	// Pass parameters as an array — never interpolate values into the SQL string.
	// Works on both connections and transactions.
	//
	// Example:
	//   rows, err = dbQuery(conn, "SELECT id, name FROM users WHERE active = ?", [true])
	//   if err != null { println(err.message)  return }
	//   for row in rows {
	//       println("{row["id"]}  {row["name"]}")
	//   }
	Builtins["dbQuery"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 {
			return runtimeError("dbQuery expects at least 2 arguments (conn, sql, ?args)", ast.Pos{})
		}
		qable, _, errObj := sqlExtract(args[0], "dbQuery")
		if errObj != nil {
			return errObj
		}
		queryStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("dbQuery: sql must be string, got %s", args[1].Type()), ast.Pos{})
		}
		sqlArgs, errObj := sqlBuildArgs(args, 2, "dbQuery")
		if errObj != nil {
			return errObj
		}
		ctx, cancel := sqlMakeContext(args[0])
		defer cancel()
		rows, err := qable.QueryContext(ctx, queryStr.Value, sqlArgs...)
		if err != nil {
			return sqlError("DB_QUERY_ERROR", err.Error())
		}
		defer rows.Close()
		result, err := sqlRowsToArray(rows)
		if err != nil {
			return sqlError("DB_SCAN_ERROR", err.Error())
		}
		return sqlOk(result)
	}}

	// dbQueryOne(conn, sql, ?args) → (row, err)
	//
	// Executes a SELECT and returns the first row as a hash, or null if no rows.
	// Use when you expect exactly one result (e.g. lookup by primary key).
	// Works on both connections and transactions.
	//
	// Example:
	//   row, err = dbQueryOne(conn, "SELECT * FROM users WHERE id = ?", [42])
	//   if err != null { println(err.message)  return }
	//   if row == null { println("not found")  return }
	//   println(row["name"])
	Builtins["dbQueryOne"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 {
			return runtimeError("dbQueryOne expects at least 2 arguments (conn, sql, ?args)", ast.Pos{})
		}
		qable, _, errObj := sqlExtract(args[0], "dbQueryOne")
		if errObj != nil {
			return errObj
		}
		queryStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("dbQueryOne: sql must be string, got %s", args[1].Type()), ast.Pos{})
		}
		sqlArgs, errObj := sqlBuildArgs(args, 2, "dbQueryOne")
		if errObj != nil {
			return errObj
		}
		ctx, cancel := sqlMakeContext(args[0])
		defer cancel()
		rows, err := qable.QueryContext(ctx, queryStr.Value, sqlArgs...)
		if err != nil {
			return sqlError("DB_QUERY_ERROR", err.Error())
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			return sqlError("DB_SCAN_ERROR", err.Error())
		}
		if !rows.Next() {
			return sqlOk(NULL)
		}
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return sqlError("DB_SCAN_ERROR", err.Error())
		}
		h := &Hash{Pairs: make(map[HashKey]HashPair, len(cols))}
		for i, col := range cols {
			key := &String{Value: col}
			hk := HashKey{Type: STRING_OBJ, Value: col}
			h.Pairs[hk] = HashPair{Key: key, Value: sqlToKLex(raw[i])}
		}
		return sqlOk(h)
	}}

	// dbExec(conn, sql, ?args) → (rowsAffected, err)
	//
	// Executes an INSERT, UPDATE, DELETE, or DDL statement.
	// Returns the number of rows affected as an integer (-1 if unavailable).
	// Pass parameters as an array — never interpolate values into the SQL string.
	// Works on both connections and transactions.
	//
	// Example:
	//   n, err = dbExec(conn, "UPDATE accounts SET balance = ? WHERE id = ?", [1500.0, 42])
	//   if err != null { println(err.message)  return }
	//   println("{n} row(s) updated")
	Builtins["dbExec"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 {
			return runtimeError("dbExec expects at least 2 arguments (conn, sql, ?args)", ast.Pos{})
		}
		qable, _, errObj := sqlExtract(args[0], "dbExec")
		if errObj != nil {
			return errObj
		}
		queryStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("dbExec: sql must be string, got %s", args[1].Type()), ast.Pos{})
		}
		sqlArgs, errObj := sqlBuildArgs(args, 2, "dbExec")
		if errObj != nil {
			return errObj
		}
		ctx, cancel := sqlMakeContext(args[0])
		defer cancel()
		result, err := qable.ExecContext(ctx, queryStr.Value, sqlArgs...)
		if err != nil {
			return sqlError("DB_EXEC_ERROR", err.Error())
		}
		affected, err := result.RowsAffected()
		if err != nil {
			affected = -1
		}
		return sqlOk(&Integer{Value: int(affected)})
	}}

	// dbOpenWithPool(driver, dsn, options) → (conn, err)
	//
	// Like dbOpen but with explicit connection pool configuration.
	// Options is a hash with any of these keys (all optional):
	//   maxIdle:     max idle connections kept in pool (default 2)
	//   maxOpen:     max open connections (0 = unlimited, default 0)
	//   idleTimeout: seconds before an idle connection is closed (default unlimited)
	//   lifetime:    max seconds a connection may be reused (default unlimited)
	//
	// Example:
	//   conn, err = dbOpenWithPool("mssql", "server=myserver;...", {
	//       maxIdle: 5, maxOpen: 20, idleTimeout: 300, lifetime: 3600
	//   })
	//   if err != null { println(err.message)  return }
	Builtins["dbOpenWithPool"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("dbOpenWithPool expects 2 or 3 arguments (driver, dsn, ?options)", ast.Pos{})
		}
		driverArg, ok1 := args[0].(*String)
		dsnArg, ok2 := args[1].(*String)
		if !ok1 {
			return typeError(fmt.Sprintf("dbOpenWithPool: driver must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if !ok2 {
			return typeError(fmt.Sprintf("dbOpenWithPool: dsn must be string, got %s", args[1].Type()), ast.Pos{})
		}
		goDriver := sqlResolveDriver(driverArg.Value)
		db, err := sql.Open(goDriver, dsnArg.Value)
		if err != nil {
			return sqlError("DB_OPEN_ERROR", err.Error())
		}
		if len(args) == 3 {
			opts, ok := args[2].(*Hash)
			if !ok {
				db.Close()
				return typeError(fmt.Sprintf("dbOpenWithPool: options must be a hash, got %s", args[2].Type()), ast.Pos{})
			}
			if pair, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "maxIdle"}]; ok {
				if iv, ok := pair.Value.(*Integer); ok {
					db.SetMaxIdleConns(iv.Value)
				}
			}
			if pair, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "maxOpen"}]; ok {
				if iv, ok := pair.Value.(*Integer); ok {
					db.SetMaxOpenConns(iv.Value)
				}
			}
			if pair, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "idleTimeout"}]; ok {
				if iv, ok := pair.Value.(*Integer); ok {
					db.SetConnMaxIdleTime(time.Duration(iv.Value) * time.Second)
				}
			}
			if pair, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "lifetime"}]; ok {
				if iv, ok := pair.Value.(*Integer); ok {
					db.SetConnMaxLifetime(time.Duration(iv.Value) * time.Second)
				}
			}
		}
		if err := db.PingContext(context.Background()); err != nil {
			db.Close()
			return sqlError("DB_CONNECT_ERROR", err.Error())
		}
		return sqlOk(&DBConn{DB: db, Driver: driverArg.Value})
	}}

	// dbBulkInsert(conn, table, columns, rows) → (n, err)
	//
	// Inserts multiple rows in a single SQL statement for maximum throughput.
	// One round trip per batch — far faster than calling dbExec in a loop.
	//
	//   table   — table name string (must be trusted — not parameterisable in SQL)
	//   columns — array of column name strings
	//   rows    — array of arrays, each sub-array is one row's values
	//
	// Auto-batches to stay within driver parameter limits:
	//   MSSQL/SQL Server — 2000 params per batch
	//   PostgreSQL        — 60000 params per batch
	//
	// Returns the total number of rows affected across all batches.
	//
	// WARNING: table and column names are interpolated directly into the SQL string.
	// Never pass user-controlled input as table or column names.
	//
	// Example:
	//   n, err = dbBulkInsert(conn, "users", ["id", "name", "age"], [
	//       [1, "Alice", 28],
	//       [2, "Bob",   35],
	//   ])
	//   if err != null { println(err.message)  return }
	//   println("{n} rows inserted")
	Builtins["dbBulkInsert"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("dbBulkInsert expects 4 arguments (conn, table, columns, rows)", ast.Pos{})
		}
		qable, driver, errObj := sqlExtract(args[0], "dbBulkInsert")
		if errObj != nil {
			return errObj
		}
		tableArg, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("dbBulkInsert: table must be string, got %s", args[1].Type()), ast.Pos{})
		}
		colsArr, ok := args[2].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("dbBulkInsert: columns must be array, got %s", args[2].Type()), ast.Pos{})
		}
		rowsArr, ok := args[3].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("dbBulkInsert: rows must be array, got %s", args[3].Type()), ast.Pos{})
		}
		if len(colsArr.Elements) == 0 {
			return runtimeError("dbBulkInsert: columns array must not be empty", ast.Pos{})
		}
		if len(rowsArr.Elements) == 0 {
			return sqlOk(&Integer{Value: 0})
		}

		numCols := len(colsArr.Elements)
		colNames := make([]string, numCols)
		for i, el := range colsArr.Elements {
			s, ok := el.(*String)
			if !ok {
				return typeError(fmt.Sprintf("dbBulkInsert: column name at index %d must be string, got %s", i, el.Type()), ast.Pos{})
			}
			colNames[i] = s.Value
		}

		isPostgres := driver == "postgres" || driver == "pgx"
		maxParams := 2000
		if isPostgres {
			maxParams = 60000
		}
		batchSize := maxParams / numCols
		if batchSize < 1 {
			batchSize = 1
		}

		colList := strings.Join(colNames, ", ")
		totalAffected := 0
		allRows := rowsArr.Elements

		for start := 0; start < len(allRows); start += batchSize {
			end := start + batchSize
			if end > len(allRows) {
				end = len(allRows)
			}
			batch := allRows[start:end]

			valueClauses := make([]string, len(batch))
			sqlArgs := make([]interface{}, 0, len(batch)*numCols)
			paramIdx := 1

			for r, rowObj := range batch {
				rowArr, ok := rowObj.(*Array)
				if !ok {
					return typeError(fmt.Sprintf("dbBulkInsert: row %d must be array, got %s", start+r, rowObj.Type()), ast.Pos{})
				}
				if len(rowArr.Elements) != numCols {
					return runtimeError(fmt.Sprintf("dbBulkInsert: row %d has %d values but %d columns declared", start+r, len(rowArr.Elements), numCols), ast.Pos{})
				}
				placeholders := make([]string, numCols)
				for c, val := range rowArr.Elements {
					if isPostgres {
						placeholders[c] = fmt.Sprintf("$%d", paramIdx)
					} else {
						placeholders[c] = "?"
					}
					paramIdx++
					sqlArgs = append(sqlArgs, kLexToSQL(val))
				}
				valueClauses[r] = "(" + strings.Join(placeholders, ", ") + ")"
			}

			query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", tableArg.Value, colList, strings.Join(valueClauses, ", "))
			batchCtx, batchCancel := sqlMakeContext(args[0])
			result, err := qable.ExecContext(batchCtx, query, sqlArgs...)
			batchCancel()
			if err != nil {
				return sqlError("DB_BULK_INSERT_ERROR", err.Error())
			}
			affected, err := result.RowsAffected()
			if err == nil {
				totalAffected += int(affected)
			}
		}

		return sqlOk(&Integer{Value: totalAffected})
	}}

	// dbQueryStream(conn, sql, ?args) → (channel, err)
	//
	// Executes a SELECT and returns a channel that yields rows one at a time.
	// Each value sent on the channel is a hash (same format as dbQuery).
	// Suitable for large result sets where loading all rows into memory is undesirable.
	// Works on both connections and transactions.
	// Break out of the for-in loop to cancel the stream early.
	//
	// Example:
	//   stream, err = dbQueryStream(conn, "SELECT id, name FROM big_table", [])
	//   if err != null { println(err.message)  return }
	//   for row in stream {
	//       println("{row["id"]}  {row["name"]}")
	//   }
	Builtins["dbQueryStream"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 {
			return runtimeError("dbQueryStream expects at least 2 arguments (conn, sql, ?args)", ast.Pos{})
		}
		qable, _, errObj := sqlExtract(args[0], "dbQueryStream")
		if errObj != nil {
			return errObj
		}
		queryStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("dbQueryStream: sql must be string, got %s", args[1].Type()), ast.Pos{})
		}
		sqlArgs, errObj := sqlBuildArgs(args, 2, "dbQueryStream")
		if errObj != nil {
			return errObj
		}
		rows, err := qable.QueryContext(context.Background(), queryStr.Value, sqlArgs...)
		if err != nil {
			return sqlError("DB_QUERY_ERROR", err.Error())
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return sqlError("DB_SCAN_ERROR", err.Error())
		}
		ch := &Channel{
			ch:   make(chan Object, 16),
			done: make(chan struct{}),
		}
		go func() {
			defer close(ch.ch)
			defer rows.Close()
			for rows.Next() {
				raw := make([]interface{}, len(cols))
				ptrs := make([]interface{}, len(cols))
				for i := range raw {
					ptrs[i] = &raw[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return
				}
				h := &Hash{Pairs: make(map[HashKey]HashPair, len(cols))}
				for i, col := range cols {
					key := &String{Value: col}
					hk := HashKey{Type: STRING_OBJ, Value: col}
					h.Pairs[hk] = HashPair{Key: key, Value: sqlToKLex(raw[i])}
				}
				select {
				case ch.ch <- h:
				case <-ch.done:
					return
				}
			}
		}()
		return sqlOk(ch)
	}}

	// dbSetTimeout(conn, ms) → null
	//
	// Sets a per-operation timeout on a connection or transaction.
	// All subsequent dbQuery, dbQueryOne, dbExec, dbBulkInsert calls on this
	// conn/tx will fail with DB_TIMEOUT_ERROR if they exceed ms milliseconds.
	// Pass 0 to remove the timeout and revert to no deadline.
	// dbBegin propagates the conn's timeout to the new transaction automatically.
	//
	// Example:
	//   dbSetTimeout(conn, 5000)   // 5 second limit per query
	//   rows, err = dbQuery(conn, "SELECT * FROM large_table", [])
	//   // → DB_TIMEOUT_ERROR if the query takes more than 5s
	Builtins["dbSetTimeout"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("dbSetTimeout expects 2 arguments (conn, ms)", ast.Pos{})
		}
		msArg, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("dbSetTimeout: ms must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		switch v := args[0].(type) {
		case *DBConn:
			v.Timeout = time.Duration(msArg.Value) * time.Millisecond
		case *DBTx:
			v.Timeout = time.Duration(msArg.Value) * time.Millisecond
		default:
			return typeError(fmt.Sprintf("dbSetTimeout: first argument must be a db connection or transaction, got %s", args[0].Type()), ast.Pos{})
		}
		return NULL
	}}

	// dbExecReturning(conn, sql, ?args) → (rows, err)
	//
	// Executes a DML statement that returns rows — for INSERT/UPDATE/DELETE
	// with RETURNING (PostgreSQL) or OUTPUT (SQL Server) clauses.
	// Returns an array of hashes, same format as dbQuery.
	// Works on both connections and transactions.
	//
	// Example (PostgreSQL):
	//   rows, err = dbExecReturning(conn, "INSERT INTO users (name) VALUES (?) RETURNING id, name", ["Alice"])
	//   if err != null { println(err.message)  return }
	//   id = rows[0]["id"]
	//
	// Example (SQL Server):
	//   rows, err = dbExecReturning(conn, "INSERT INTO users (name) OUTPUT INSERTED.id, INSERTED.name VALUES (?)", ["Alice"])
	//   if err != null { println(err.message)  return }
	//   id = rows[0]["id"]
	Builtins["dbExecReturning"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 {
			return runtimeError("dbExecReturning expects at least 2 arguments (conn, sql, ?args)", ast.Pos{})
		}
		qable, _, errObj := sqlExtract(args[0], "dbExecReturning")
		if errObj != nil {
			return errObj
		}
		queryStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("dbExecReturning: sql must be string, got %s", args[1].Type()), ast.Pos{})
		}
		sqlArgs, errObj := sqlBuildArgs(args, 2, "dbExecReturning")
		if errObj != nil {
			return errObj
		}
		ctx, cancel := sqlMakeContext(args[0])
		defer cancel()
		rows, err := qable.QueryContext(ctx, queryStr.Value, sqlArgs...)
		if err != nil {
			return sqlError("DB_EXEC_ERROR", err.Error())
		}
		defer rows.Close()
		result, err := sqlRowsToArray(rows)
		if err != nil {
			return sqlError("DB_SCAN_ERROR", err.Error())
		}
		return sqlOk(result)
	}}

	// dbBegin(conn) → (tx, err)
	//
	// Starts a database transaction. Use dbCommit or dbRollback to finish it.
	// Pass the returned tx to dbQuery, dbExec, dbCommit, and dbRollback.
	//
	// Example:
	//   tx, err = dbBegin(conn)
	//   if err != null { return }
	//   _, err = dbExec(tx, "INSERT INTO log VALUES (?, ?)", [42, "updated"])
	//   _, err = dbExec(tx, "UPDATE accounts SET balance = ? WHERE id = ?", [1500, 42])
	//   if err != null { dbRollback(tx)  return }
	//   dbCommit(tx)
	Builtins["dbBegin"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("dbBegin expects 1 argument (conn)", ast.Pos{})
		}
		conn, ok := args[0].(*DBConn)
		if !ok {
			return typeError(fmt.Sprintf("dbBegin: argument must be a db connection, got %s", args[0].Type()), ast.Pos{})
		}
		ctx, cancel := sqlMakeContext(conn)
		defer cancel()
		tx, err := conn.DB.BeginTx(ctx, nil)
		if err != nil {
			return sqlError("DB_BEGIN_ERROR", err.Error())
		}
		return sqlOk(&DBTx{Tx: tx, Driver: conn.Driver, Timeout: conn.Timeout})
	}}

	// dbCommit(tx) → (null, err)
	//
	// Commits a transaction started with dbBegin.
	Builtins["dbCommit"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("dbCommit expects 1 argument (tx)", ast.Pos{})
		}
		tx, ok := args[0].(*DBTx)
		if !ok {
			return typeError(fmt.Sprintf("dbCommit: argument must be a db transaction, got %s", args[0].Type()), ast.Pos{})
		}
		if err := tx.Tx.Commit(); err != nil {
			return sqlError("DB_COMMIT_ERROR", err.Error())
		}
		return sqlOk(NULL)
	}}

	// dbRollback(tx) → (null, err)
	//
	// Rolls back a transaction started with dbBegin.
	// Safe to call even if the transaction has already been committed or rolled back.
	Builtins["dbRollback"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("dbRollback expects 1 argument (tx)", ast.Pos{})
		}
		tx, ok := args[0].(*DBTx)
		if !ok {
			return typeError(fmt.Sprintf("dbRollback: argument must be a db transaction, got %s", args[0].Type()), ast.Pos{})
		}
		tx.Tx.Rollback()
		return NULL
	}}
}
