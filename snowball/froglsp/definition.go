package main

import "klex/ast"

// DefinitionAtPosition returns the location of the definition for the symbol at the given position.
// Cross-file navigation (module.symbol) is handled first; same-file identifiers fall through.
func DefinitionAtPosition(doc *DocumentState, pos Position) interface{} {
	if doc.AST == nil || doc.Symbols == nil {
		return nil
	}

	// Check for DotExpr: module.symbol requires cross-file resolution.
	node := FindNodeAtPosition(doc.AST, pos.Line, pos.Character)
	if node != nil {
		var dotExpr *ast.DotExpr
		switch n := node.(type) {
		case *ast.DotExpr:
			dotExpr = n
		case *ast.CallExpr:
			if d, ok := n.Function.(*ast.DotExpr); ok {
				dotExpr = d
			}
		}
		if dotExpr != nil {
			if ident, ok := dotExpr.Left.(*ast.Ident); ok {
				libFile, _, sym, ok := resolveModuleSymbol(doc, ident.Value, dotExpr.Property)
				if ok {
					return Location{
						URI: PathToURI(libFile),
						Range: Range{
							Start: Position{Line: sym.DefPos.Line - 1, Character: sym.DefPos.Col - 1},
							End:   Position{Line: sym.DefPos.Line - 1, Character: sym.DefPos.Col - 1 + len(sym.Name)},
						},
					}
				}
			}
			// DotExpr but couldn't resolve (struct field, unresolvable import) — don't fall through.
			return nil
		}
	}

	// Plain identifier — same-file symbol lookup.
	ident := GetIdentAtPosition(doc.AST, pos.Line, pos.Character)
	if ident == nil {
		return nil
	}
	sym, ok := doc.Symbols.Symbols[ident.Value]
	if !ok {
		return nil
	}
	return Location{
		URI: sym.DefURI,
		Range: Range{
			Start: Position{Line: sym.DefPos.Line - 1, Character: sym.DefPos.Col - 1},
			End:   Position{Line: sym.DefPos.Line - 1, Character: sym.DefPos.Col - 1 + len(sym.Name)},
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
