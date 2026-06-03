package eval

import (
	"context"
	"database/sql"
	"fmt"
	"klex/ast"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"  // registers "pgx" driver
	_ "github.com/microsoft/go-mssqldb" // registers "mssql" and "sqlserver" drivers
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
	// dbOpen — open a database connection pool and verify it with a ping.
	//
	// The entry point to the database API. Returns a pooled connection that's safe
	// to share across async tasks — open it once and reuse it. Supported drivers:
	// "mssql" (Microsoft SQL Server; also accepts "sqlserver") and "postgres"
	// (PostgreSQL via pgx). The dsn is the driver's native connection string, e.g.
	// "server=host;database=mydb;user id=sa;password=Pass1!" for mssql or
	// "postgres://user:pass@host:5432/mydb" for postgres. Always pass query
	// parameters to dbQuery/dbExec as an array — never interpolate values into SQL.
	//
	// @sig     dbOpen(driver: string, dsn: string) -> (connection, error)
	// @param   driver  "mssql"/"sqlserver" or "postgres"
	// @param   dsn     the driver's connection string
	// @returns a (connection, null) tuple on success, or (null, error) on failure
	// @errors  TypeError if driver or dsn isn't a string; returns DB_OPEN_ERROR / DB_CONNECT_ERROR in the tuple's second slot
	// @example no-run conn, err = dbOpen("postgres", "postgres://user:pass@host:5432/mydb")
	// @since   0.1.0
	// @see     dbOpenWithPool, dbQuery, dbExec, dbClose
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

	// dbClose — close a database connection pool.
	//
	// Releases all pooled connections. Best-effort; call it when you're done with
	// the connection so the database doesn't keep idle sessions open.
	//
	// @sig     dbClose(conn: connection) -> null
	// @param   conn  the connection to close
	// @returns null
	// @errors  TypeError if the argument isn't a db connection
	// @example no-run dbClose(conn)
	// @since   0.1.0
	// @see     dbOpen, dbPing
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

	// dbPing — check that a database connection is still alive.
	//
	// Round-trips to the server to confirm the connection works — the basis of a
	// health check. Follows the two-path pattern: (null, null) when healthy,
	// (null, error) when the ping fails.
	//
	// @sig     dbPing(conn: connection) -> (null, error)
	// @param   conn  the connection to test
	// @returns (null, null) if alive, or (null, error) if the ping fails
	// @errors  TypeError if the argument isn't a db connection; returns DB_PING_ERROR in the tuple's second slot
	// @example no-run _, err = dbPing(conn)
	// @since   0.1.0
	// @see     dbOpen, dbClose
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

	// dbQuery — run a SELECT and return every row as an array of hashes.
	//
	// Each row is a hash keyed by column name; SQL NULLs become kLex null. Pass
	// query parameters as an array (the `?` placeholders) — never interpolate
	// values into the SQL string, both to avoid injection and to let the driver
	// type them correctly. Works on a connection or a transaction.
	//
	// @sig     dbQuery(conn: connection|transaction, sql: string, [args: array]) -> (array, error)
	// @param   conn  a connection or transaction
	// @param   sql   the SELECT statement, with ? placeholders for parameters
	// @param   args  an array of parameter values, one per placeholder (optional)
	// @returns an (array-of-row-hashes, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns DB_QUERY_ERROR / DB_SCAN_ERROR in the tuple's second slot
	// @example no-run rows, err = dbQuery(conn, "SELECT id, name FROM users WHERE active = ?", [true])
	// @since   0.1.0
	// @see     dbQueryOne, dbQueryStream, dbExec
	Builtins["dbQuery"] = &Builtin{Fn: func(args []Object) Object {
		// OFI #19b (2026-05-23): upper-bound the arity. sqlBuildArgs
		// only ever reads args[2] — anything past it was silently
		// swallowed (e.g. dbQuery(conn, sql, [1,2,3], "extra") used
		// to succeed and ignore the trailing arg). Now matches the
		// LSP signature `dbQuery(conn, sql, args?: array)`.
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("dbQuery expects 2 or 3 arguments (conn, sql, ?args)", ast.Pos{})
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

	// dbQueryOne — run a SELECT and return just the first row, or null.
	//
	// Like dbQuery but returns a single row hash instead of an array — for lookups
	// you expect to match one record (e.g. by primary key). Returns null (with a
	// null error) when no row matches, so check for that before indexing the hash.
	// Works on a connection or a transaction.
	//
	// @sig     dbQueryOne(conn: connection|transaction, sql: string, [args: array]) -> (hash, error)
	// @param   conn  a connection or transaction
	// @param   sql   the SELECT statement, with ? placeholders for parameters
	// @param   args  an array of parameter values, one per placeholder (optional)
	// @returns a (row-hash, null) tuple, (null, null) if no rows matched, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns DB_QUERY_ERROR / DB_SCAN_ERROR in the tuple's second slot
	// @example no-run row, err = dbQueryOne(conn, "SELECT * FROM users WHERE id = ?", [42])
	// @since   0.1.0
	// @see     dbQuery, dbExec
	Builtins["dbQueryOne"] = &Builtin{Fn: func(args []Object) Object {
		// OFI #19b: see dbQuery for the rationale.
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("dbQueryOne expects 2 or 3 arguments (conn, sql, ?args)", ast.Pos{})
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

	// dbExec — run an INSERT, UPDATE, DELETE, or DDL statement.
	//
	// For statements that change data rather than return rows. Returns the number
	// of rows affected (-1 if the driver can't report it). Pass parameters as an
	// array — never interpolate values into the SQL string. Works on a connection
	// or a transaction. If you need the rows a write produces (RETURNING/OUTPUT),
	// use dbExecReturning instead.
	//
	// @sig     dbExec(conn: connection|transaction, sql: string, [args: array]) -> (int, error)
	// @param   conn  a connection or transaction
	// @param   sql   the statement, with ? placeholders for parameters
	// @param   args  an array of parameter values, one per placeholder (optional)
	// @returns a (rowsAffected, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns DB_EXEC_ERROR in the tuple's second slot
	// @example no-run n, err = dbExec(conn, "UPDATE accounts SET balance = ? WHERE id = ?", [1500.0, 42])
	// @since   0.1.0
	// @see     dbExecReturning, dbQuery, dbBulkInsert
	Builtins["dbExec"] = &Builtin{Fn: func(args []Object) Object {
		// OFI #19b: see dbQuery for the rationale.
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("dbExec expects 2 or 3 arguments (conn, sql, ?args)", ast.Pos{})
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

	// dbOpenWithPool — open a database connection with explicit pool tuning.
	//
	// Like dbOpen, but the optional options hash configures the connection pool:
	// "maxIdle" (idle connections kept, default 2), "maxOpen" (max open, 0 =
	// unlimited), "idleTimeout" (seconds before an idle connection is closed),
	// "lifetime" (max seconds a connection may be reused). Use it to size the pool
	// for a busy service or to bound connection lifetimes behind a load balancer.
	//
	// @sig     dbOpenWithPool(driver: string, dsn: string, [options: hash]) -> (connection, error)
	// @param   driver   "mssql"/"sqlserver" or "postgres"
	// @param   dsn      the driver's connection string
	// @param   options  pool settings: maxIdle / maxOpen / idleTimeout / lifetime (optional)
	// @returns a (connection, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns DB_OPEN_ERROR / DB_CONNECT_ERROR in the tuple's second slot
	// @example no-run conn, err = dbOpenWithPool("mssql", dsn, {"maxIdle": 5, "maxOpen": 20})
	// @since   0.1.0
	// @see     dbOpen, dbSetTimeout, dbClose
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

	// dbBulkInsert — insert many rows in batched multi-row statements, fast.
	//
	// Builds multi-row INSERTs so the whole load is a handful of round trips, far
	// faster than dbExec in a loop. `columns` is an array of column names; `rows`
	// is an array of arrays, each a row's values in column order. Auto-batches to
	// stay within driver parameter limits (MSSQL ~2000, PostgreSQL ~60000 params).
	// Returns the total rows affected. WARNING: the table and column names are
	// interpolated straight into the SQL — never pass user-controlled input there
	// (the row values are safely parameterised; the identifiers are not).
	//
	// @sig     dbBulkInsert(conn: connection|transaction, table: string, columns: array, rows: array) -> (int, error)
	// @param   conn     a connection or transaction
	// @param   table    the target table name (must be trusted)
	// @param   columns  an array of column-name strings (must be trusted, non-empty)
	// @param   rows     an array of row arrays, each holding one value per column
	// @returns a (rowsInserted, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; RuntimeError if columns is empty; returns DB_EXEC_ERROR in the tuple's second slot
	// @example no-run n, err = dbBulkInsert(conn, "users", ["id", "name"], [[1, "Alice"], [2, "Bob"]])
	// @since   0.1.0
	// @see     dbExec, dbExecReturning
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

	// dbQueryStream — run a SELECT and stream rows one at a time over a channel.
	//
	// The streaming counterpart to dbQuery: returns a channel that yields each row
	// (a hash, same shape as dbQuery) as it arrives, so you can process a huge
	// result set without holding it all in memory. Drain it with for-in; `break`
	// cancels the stream early and releases the underlying cursor. Works on a
	// connection or a transaction.
	//
	// @sig     dbQueryStream(conn: connection|transaction, sql: string, [args: array]) -> (channel, error)
	// @param   conn  a connection or transaction
	// @param   sql   the SELECT statement, with ? placeholders for parameters
	// @param   args  an array of parameter values, one per placeholder (optional)
	// @returns a (channel-of-row-hashes, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns DB_QUERY_ERROR in the tuple's second slot
	// @example no-run for row in first(dbQueryStream(conn, "SELECT id FROM big_table", [])) { print(row["id"]) }
	// @since   0.1.0
	// @see     dbQuery, dbQueryOne
	Builtins["dbQueryStream"] = &Builtin{Fn: func(args []Object) Object {
		// OFI #19b: see dbQuery for the rationale.
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("dbQueryStream expects 2 or 3 arguments (conn, sql, ?args)", ast.Pos{})
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

	// dbSetTimeout — set a per-operation timeout on a connection or transaction.
	//
	// After this, every dbQuery / dbQueryOne / dbExec / dbBulkInsert on the same
	// conn or tx fails with DB_TIMEOUT_ERROR if it runs longer than `ms`
	// milliseconds — a guard against a runaway query hanging your program. Pass 0
	// to remove the limit. dbBegin copies the connection's timeout onto the new
	// transaction automatically.
	//
	// @sig     dbSetTimeout(conn: connection|transaction, ms: int) -> null
	// @param   conn  the connection or transaction to configure
	// @param   ms    the per-operation timeout in milliseconds (0 to disable)
	// @returns null
	// @errors  TypeError if ms isn't an integer or conn isn't a connection/transaction
	// @example no-run dbSetTimeout(conn, 5000)
	// @since   0.1.0
	// @see     dbOpenWithPool, dbQuery, dbExec
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

	// dbExecReturning — run a write that returns rows (RETURNING / OUTPUT).
	//
	// For an INSERT/UPDATE/DELETE that also produces rows — PostgreSQL's RETURNING
	// or SQL Server's OUTPUT clause — typically to read back generated IDs. Returns
	// an array of row hashes, the same shape as dbQuery. Use plain dbExec when you
	// only need the affected-row count. Works on a connection or a transaction.
	//
	// @sig     dbExecReturning(conn: connection|transaction, sql: string, [args: array]) -> (array, error)
	// @param   conn  a connection or transaction
	// @param   sql   the DML statement with a RETURNING/OUTPUT clause and ? placeholders
	// @param   args  an array of parameter values, one per placeholder (optional)
	// @returns an (array-of-row-hashes, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns DB_EXEC_ERROR / DB_SCAN_ERROR in the tuple's second slot
	// @example no-run rows, err = dbExecReturning(conn, "INSERT INTO users (name) VALUES (?) RETURNING id", ["Alice"])
	// @since   0.1.0
	// @see     dbExec, dbQuery
	Builtins["dbExecReturning"] = &Builtin{Fn: func(args []Object) Object {
		// OFI #19b: see dbQuery for the rationale.
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("dbExecReturning expects 2 or 3 arguments (conn, sql, ?args)", ast.Pos{})
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

	// dbBegin — start a database transaction.
	//
	// Returns a transaction handle you pass to dbQuery / dbExec instead of the
	// connection, so the statements run atomically — finish with dbCommit to apply
	// them or dbRollback to discard them. The transaction inherits the connection's
	// timeout (see dbSetTimeout).
	//
	// @sig     dbBegin(conn: connection) -> (transaction, error)
	// @param   conn  the connection to begin a transaction on
	// @returns a (transaction, null) tuple on success, or (null, error) on failure
	// @errors  TypeError if the argument isn't a db connection; returns DB_BEGIN_ERROR in the tuple's second slot
	// @example no-run tx, err = dbBegin(conn)
	// @since   0.1.0
	// @see     dbCommit, dbRollback, dbExec
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

	// dbCommit — commit a transaction, making its changes permanent.
	//
	// Applies everything done on the transaction since dbBegin. After commit the
	// transaction handle is spent — don't reuse it. Two-path return: (null, null)
	// on success, (null, error) if the commit fails.
	//
	// @sig     dbCommit(tx: transaction) -> (null, error)
	// @param   tx  the transaction to commit
	// @returns (null, null) on success, or (null, error) on failure
	// @errors  TypeError if the argument isn't a db transaction; returns DB_COMMIT_ERROR in the tuple's second slot
	// @example no-run _, err = dbCommit(tx)
	// @since   0.1.0
	// @see     dbBegin, dbRollback
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

	// dbRollback — discard a transaction's changes.
	//
	// Undoes everything done since dbBegin — the escape hatch when an error mid-
	// transaction means the whole unit of work should not apply. Safe to call even
	// if the transaction was already committed or rolled back, so it's the natural
	// thing to call on any error path. Returns null.
	//
	// @sig     dbRollback(tx: transaction) -> null
	// @param   tx  the transaction to roll back
	// @returns null
	// @errors  TypeError if the argument isn't a db transaction
	// @example no-run dbRollback(tx)
	// @since   0.1.0
	// @see     dbBegin, dbCommit
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
