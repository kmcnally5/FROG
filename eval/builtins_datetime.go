package eval

import (
	"fmt"
	"klex/ast"
	"time"
)

// timeToTuple converts a time.Time to the 7-element field tuple used by
// _timeFields. Kept as a helper to avoid duplicating the field extraction.
func timeToTuple(t time.Time) *Tuple {
	return &Tuple{Elements: []Object{
		&Integer{Value: t.Year()},
		&Integer{Value: int(t.Month())},
		&Integer{Value: t.Day()},
		&Integer{Value: t.Hour()},
		&Integer{Value: t.Minute()},
		&Integer{Value: t.Second()},
		&String{Value: t.Weekday().String()},
	}}
}

func init() {
	// _timeNanos() → integer
	// Returns the current time in nanoseconds since an arbitrary epoch.
	// Suitable for high-resolution timing and benchmarking.
	Builtins["_timeNanos"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_timeNanos expects no arguments", ast.Pos{})
		}
		return &Integer{Value: int(time.Now().UnixNano())}
	}}

	// _timeNow() → (year, month, day, hour, minute, second, unix, weekday)
	// Returns the current local time as an 8-element tuple.
	// unix is integer seconds since 1970-01-01 00:00:00 UTC.
	// weekday is the full English name: "Monday", "Tuesday", etc.
	Builtins["_timeNow"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_timeNow expects no arguments", ast.Pos{})
		}
		t := time.Now()
		return &Tuple{Elements: []Object{
			&Integer{Value: t.Year()},
			&Integer{Value: int(t.Month())},
			&Integer{Value: t.Day()},
			&Integer{Value: t.Hour()},
			&Integer{Value: t.Minute()},
			&Integer{Value: t.Second()},
			&Integer{Value: int(t.Unix())},
			&String{Value: t.Weekday().String()},
		}}
	}}

	// _timeFields(unix) → (year, month, day, hour, minute, second, weekday)
	// Converts a unix timestamp (integer seconds) into local time fields.
	Builtins["_timeFields"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_timeFields expects 1 argument", ast.Pos{})
		}
		u, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_timeFields: argument must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		return timeToTuple(time.Unix(int64(u.Value), 0))
	}}

	// _timeFormat(unix, layout) → string
	// Formats a unix timestamp using Go's reference-time layout string.
	// Named layout constants are provided in datetime.lex for convenience.
	Builtins["_timeFormat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_timeFormat expects 2 arguments", ast.Pos{})
		}
		u, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_timeFormat: first argument must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		layout, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_timeFormat: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		return &String{Value: time.Unix(int64(u.Value), 0).Format(layout.Value)}
	}}

	// _timeParse(str, layout) → (unix, err)
	// Parses a time string using Go's reference-time layout. Returns the unix
	// timestamp on success, or (0, err_message) on failure.
	Builtins["_timeParse"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_timeParse expects 2 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_timeParse: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		layout, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_timeParse: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		t, err := time.Parse(layout.Value, s.Value)
		if err != nil {
			return &Tuple{Elements: []Object{&Integer{Value: 0}, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(t.Unix())}, NULL}}
	}}
}
