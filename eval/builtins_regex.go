package eval

import (
	"fmt"
	"klex/ast"
	"regexp"
)

func init() {
	// _regexMatch(pattern, str) → (bool, err)
	// True if pattern matches anywhere in str.
	Builtins["_regexMatch"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_regexMatch expects 2 arguments", ast.Pos{})
		}
		pat, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexMatch: pattern must be string, got %s", args[0].Type()), ast.Pos{})
		}
		str, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexMatch: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		re, err := regexp.Compile(pat.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&Boolean{Value: re.MatchString(str.Value)}, NULL}}
	}}

	// _regexFind(pattern, str) → (string|null, err)
	// Returns the first match, or null if none.
	Builtins["_regexFind"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_regexFind expects 2 arguments", ast.Pos{})
		}
		pat, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexFind: pattern must be string, got %s", args[0].Type()), ast.Pos{})
		}
		str, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexFind: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		re, err := regexp.Compile(pat.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		m := re.FindString(str.Value)
		if m == "" && !re.MatchString(str.Value) {
			return &Tuple{Elements: []Object{NULL, NULL}}
		}
		return &Tuple{Elements: []Object{&String{Value: m}, NULL}}
	}}

	// _regexFindAll(pattern, str) → (array, err)
	// Returns all non-overlapping matches.
	Builtins["_regexFindAll"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_regexFindAll expects 2 arguments", ast.Pos{})
		}
		pat, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexFindAll: pattern must be string, got %s", args[0].Type()), ast.Pos{})
		}
		str, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexFindAll: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		re, err := regexp.Compile(pat.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		matches := re.FindAllString(str.Value, -1)
		elems := make([]Object, len(matches))
		for i, m := range matches {
			elems[i] = &String{Value: m}
		}
		return &Tuple{Elements: []Object{&Array{Elements: elems}, NULL}}
	}}

	// _regexReplace(pattern, str, repl) → (string, err)
	// Replaces the first match with repl.
	Builtins["_regexReplace"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_regexReplace expects 3 arguments", ast.Pos{})
		}
		pat, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexReplace: pattern must be string, got %s", args[0].Type()), ast.Pos{})
		}
		str, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexReplace: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		repl, ok := args[2].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexReplace: repl must be string, got %s", args[2].Type()), ast.Pos{})
		}
		re, err := regexp.Compile(pat.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		loc := re.FindStringIndex(str.Value)
		var result string
		if loc == nil {
			result = str.Value
		} else {
			result = str.Value[:loc[0]] + re.ReplaceAllString(str.Value[loc[0]:loc[1]], repl.Value) + str.Value[loc[1]:]
		}
		return &Tuple{Elements: []Object{&String{Value: result}, NULL}}
	}}

	// _regexReplaceAll(pattern, str, repl) → (string, err)
	// Replaces all matches with repl.
	Builtins["_regexReplaceAll"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_regexReplaceAll expects 3 arguments", ast.Pos{})
		}
		pat, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexReplaceAll: pattern must be string, got %s", args[0].Type()), ast.Pos{})
		}
		str, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexReplaceAll: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		repl, ok := args[2].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexReplaceAll: repl must be string, got %s", args[2].Type()), ast.Pos{})
		}
		re, err := regexp.Compile(pat.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: re.ReplaceAllString(str.Value, repl.Value)}, NULL}}
	}}

	// _regexSplit(pattern, str) → (array, err)
	// Splits str on every match of pattern.
	Builtins["_regexSplit"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_regexSplit expects 2 arguments", ast.Pos{})
		}
		pat, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexSplit: pattern must be string, got %s", args[0].Type()), ast.Pos{})
		}
		str, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexSplit: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		re, err := regexp.Compile(pat.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		parts := re.Split(str.Value, -1)
		elems := make([]Object, len(parts))
		for i, p := range parts {
			elems[i] = &String{Value: p}
		}
		return &Tuple{Elements: []Object{&Array{Elements: elems}, NULL}}
	}}

	// _regexGroups(pattern, str) → (array|null, err)
	// Returns capture groups of the first match. Index 0 is the full match,
	// indexes 1+ are the capture groups. Returns null if no match.
	Builtins["_regexGroups"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_regexGroups expects 2 arguments", ast.Pos{})
		}
		pat, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexGroups: pattern must be string, got %s", args[0].Type()), ast.Pos{})
		}
		str, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexGroups: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		re, err := regexp.Compile(pat.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		sub := re.FindStringSubmatch(str.Value)
		if sub == nil {
			return &Tuple{Elements: []Object{NULL, NULL}}
		}
		elems := make([]Object, len(sub))
		for i, s := range sub {
			elems[i] = &String{Value: s}
		}
		return &Tuple{Elements: []Object{&Array{Elements: elems}, NULL}}
	}}

	// _regexGroupsAll(pattern, str) → (array_of_arrays, err)
	// Returns capture groups for every match. Each element is an array where
	// index 0 is the full match and indexes 1+ are the capture groups.
	Builtins["_regexGroupsAll"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_regexGroupsAll expects 2 arguments", ast.Pos{})
		}
		pat, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexGroupsAll: pattern must be string, got %s", args[0].Type()), ast.Pos{})
		}
		str, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_regexGroupsAll: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		re, err := regexp.Compile(pat.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		allSubs := re.FindAllStringSubmatch(str.Value, -1)
		outer := make([]Object, len(allSubs))
		for i, sub := range allSubs {
			inner := make([]Object, len(sub))
			for j, s := range sub {
				inner[j] = &String{Value: s}
			}
			outer[i] = &Array{Elements: inner}
		}
		return &Tuple{Elements: []Object{&Array{Elements: outer}, NULL}}
	}}
}
