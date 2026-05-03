package main

import "klex/ast"

// DefinitionAtPosition returns the location of the definition for the symbol at the given position
func DefinitionAtPosition(doc *DocumentState, pos Position) interface{} {
	if doc.AST == nil || doc.Symbols == nil {
		return nil
	}

	// Find the identifier at this position
	ident := GetIdentAtPosition(doc.AST, pos.Line, pos.Character)
	if ident == nil {
		return nil
	}

	// Look up the symbol
	sym, ok := doc.Symbols.Symbols[ident.Value]
	if !ok {
		// Builtin — no definition location
		return nil
	}

	// Return the location of the definition
	return Location{
		URI: sym.DefURI,
		Range: Range{
			Start: Position{
				Line:      sym.DefPos.Line - 1,
				Character: sym.DefPos.Col - 1,
			},
			End: Position{
				Line:      sym.DefPos.Line - 1,
				Character: sym.DefPos.Col - 1 + len(sym.Name),
			},
		},
	}
}

// FindDefinitionsOfSymbol finds all places where a symbol is defined (Phase 2: unused, for completeness)
func FindDefinitionsOfSymbol(program *ast.Program, name string) []Location {
	var locations []Location
	// TODO: multi-file support
	return locations
}

// CrossFileDefinition resolves a definition in another file (Phase 2)
func CrossFileDefinition(currentDocURI, importPath, symbolName string) *Location {
	// TODO: when we implement cross-file support
	return nil
}
