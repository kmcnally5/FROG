if exists("b:current_syntax")
  finish
endif

" Control flow keywords
syntax keyword klexKeyword    if else while for in return break continue switch case default select

" Declaration keywords
syntax keyword klexDecl       fn struct enum import as let const

" Language self reference (struct methods)
syntax keyword klexSelf       self

" Language constants
syntax keyword klexConstant   null true false

" Built-in functions
syntax keyword klexBuiltin    println print len push pop
syntax keyword klexBuiltin    type str int float
syntax keyword klexBuiltin    split join substr slice
syntax keyword klexBuiltin    keys values hasKey delete
syntax keyword klexBuiltin    filter map reduce
syntax keyword klexBuiltin    upper lower trim replace indexOf startsWith endsWith
syntax keyword klexBuiltin    range format
syntax keyword klexBuiltin    env readFile writeFile appendFile exec input
syntax keyword klexBuiltin    channel send recv close cancel isError
syntax keyword klexBuiltin    sleep async await safe error assert

" Integers and floats
syntax match  klexNumber      "\b[0-9]\+\(\.[0-9]\+\)\?\b"

" Plain and interpolated double-quoted strings
syntax region klexString      start='"' end='"' oneline contains=klexEscape,klexInterp
syntax match  klexEscape      contained "\\[nrtb\"\\{]"
syntax region klexInterp      contained start="{" end="}" contains=@klexExpr

" Raw backtick strings — no escapes, no interpolation
syntax region klexRawString   start='`' end='`'

" Comments — // to end of line
syntax match  klexComment     "//.*$"

" Operators
syntax match  klexOperator    "==\|!=\|<=\|>=\|&&\||||\|>\|\.\.\."
syntax match  klexOperator    "[+\-*/<>!%]"
syntax match  klexAssign      "=\ze[^=]"

" Expression cluster used inside interpolation regions
syntax cluster klexExpr contains=klexKeyword,klexDecl,klexConstant,klexBuiltin,klexNumber,klexString,klexOperator

highlight def link klexKeyword    Keyword
highlight def link klexDecl       Keyword
highlight def link klexSelf       Special
highlight def link klexConstant   Constant
highlight def link klexBuiltin    Function
highlight def link klexNumber     Number
highlight def link klexString     String
highlight def link klexRawString  String
highlight def link klexEscape     SpecialChar
highlight def link klexInterp     PreProc
highlight def link klexComment    Comment
highlight def link klexOperator   Operator
highlight def link klexAssign     Operator

let b:current_syntax = "klex"
