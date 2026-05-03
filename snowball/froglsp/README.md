# froglsp — kLex Language Server Protocol (LSP) Implementation

**froglsp** is a Language Server Protocol implementation for the kLex programming language. It provides real-time language features for kLex code in supported editors, including syntax highlighting, error diagnostics, code completion, hover tooltips, and go-to-definition.

## Features

- **Syntax Highlighting** — TextMate grammar-based syntax coloring
- **Error Diagnostics** — Real-time parsing and type-checking errors
- **Code Completion** — Context-aware suggestions for builtins and symbols
- **Hover Information** — Inline documentation for functions and builtins
- **Go-to-Definition** — Jump to function/variable definitions
- **Symbol Navigation** — Find functions and variables in the current file

## Prerequisites

- **Go 1.16 or later** — required to build the LSP server
- **VS Code 1.45+** (for VS Code integration)
- A terminal or command line to build and run the server

## Installation & Setup

### 1. Build the froglsp Server

From the project root:

```bash
go build -o froglsp ./snowball/froglsp
```

This creates a `froglsp` binary in the project root. This binary runs as an LSP server over stdin/stdout, communicating with your editor via JSON-RPC 2.0.

### 2. Install VS Code Configuration Files

The LSP server requires two configuration files in your `.vscode` directory:

- **`settings.json`** — Theme customizations for syntax highlighting
- **`launch.json`** — Debug configuration for running the extension host

These files are provided in **`docs/vscode_profile/`** and must be copied to **`.vscode/`** in the project root:

```bash
# Create .vscode directory if it doesn't exist
mkdir -p .vscode

# Copy configuration files
cp docs/vscode_profile/settings.json .vscode/
cp docs/vscode_profile/launch.json .vscode/
```

### 3. Install the VS Code Extension

The extension code lives in **`editors/vscode_froglsp/klex-language/`**. To install:

1. Open the kLex project in VS Code
2. Go to **Run → Open Configurations** and select **"kLex LSP Extension"**
3. Press **F5** to launch the extension host
4. A new VS Code window will open with the extension active

The extension will automatically connect to the `froglsp` server binary and provide language features for `.lex` files.

## Configuration Files

### `settings.json`

Defines TextMate token scopes and color assignments for kLex syntax. Includes rules for:
- Keywords and control flow (purple)
- Functions and builtins (cyan)
- Strings and escape sequences (red/cyan)
- Comments (green, italicized)
- Constants and numbers (yellow/orange)

Customize colors by editing the `foreground` and `fontStyle` properties in the `textMateRules` array.

### `launch.json`

Configures VS Code's **Extension Host** for development:
- `name` — Display name in the debug configuration dropdown
- `args` — Points to the extension path (`editors/vscode_froglsp/klex-language`)
- `preLaunchTask` — Runs `npm: build` before launching (compiles TypeScript)
- `presentation` — Controls debug panel behavior

## Runtime

Once installed, the froglsp server:

1. **Listens on stdin** for JSON-RPC 2.0 messages from the editor
2. **Parses kLex code** and builds an AST
3. **Performs analysis** (type checking, symbol extraction, diagnostics)
4. **Sends results back** via stdout (hover info, completions, diagnostics, etc.)
5. **Maintains document state** — tracks open files and their versions

The server runs as a **single persistent process** per editor session, reusing parsed ASTs and symbol tables for performance.

## Development & Debugging

### Running the Extension in Debug Mode

1. Ensure `.vscode/launch.json` is in place
2. Press **F5** in VS Code to start the Extension Host
3. A new window opens with the LSP extension active
4. Set breakpoints in the extension code (`editors/vscode_froglsp/klex-language/src/`)
5. Open a `.lex` file to trigger language features

### Rebuild After Changes

If you modify the froglsp server code:

```bash
go build -o froglsp ./snowball/froglsp
```

Kill and restart the Extension Host (Ctrl+Shift+F5 or stop and F5).

If you modify the extension client code:

```bash
cd editors/vscode_froglsp/klex-language
npm run build
```

Then restart the Extension Host.

## Troubleshooting

### Extension doesn't start or shows errors
- Ensure `go build -o froglsp ./snowball/froglsp` succeeded and the binary exists at project root
- Check that `.vscode/launch.json` points to the correct extension path
- Verify Node.js and npm are installed: `npm --version`

### Language features not appearing
- Confirm the file extension is `.lex`
- Open the **Output** panel (Ctrl+K Ctrl+H) and select **"kLex Language Server"** from the dropdown
- Check for error messages in the output
- Verify the froglsp binary is executable: `chmod +x froglsp` (on macOS/Linux)

### Syntax highlighting not working
- Confirm `.vscode/settings.json` is in place
- Reload the window (Ctrl+Shift+P → **Developer: Reload Window**)
- Check the **Output** tab for diagnostics

### Server crashes or becomes unresponsive
- Restart the Extension Host (Ctrl+Shift+F5)
- Review the Output panel for panic or error messages
- Report issues with reproducible examples (minimal `.lex` files that trigger the problem)

## Architecture

**froglsp** is structured as:

- **`main.go`** — Entry point; creates transport and server
- **`transport.go`** — JSON-RPC 2.0 message serialization over stdin/stdout
- **`protocol.go`** — RPC message types (request/response/notification) and LSP types
- **`server.go`** — Handles LSP lifecycle (initialize, didOpen, didChange, etc.)
- **`analysis.go`** — Parses kLex code, extracts symbols, generates diagnostics
- **`diagnostics.go`** — Formats and reports errors to the editor
- **`hover.go`** — Generates hover information and documentation
- **`completion.go`** — Provides code completion suggestions
- **`definition.go`** — Implements go-to-definition
- **`builtins.go`** — Metadata and documentation for kLex builtins

## Future Work

- **Multi-file support** — currently single-file only; cross-file analysis planned
- **Quick fixes** — automated code corrections for common errors
- **Enhanced diagnostics** — detection of unused variables, unreachable code

## License

This is part of the kLex learning project. MIT License.

---

**Questions or issues?** Open an issue on GitHub or review the [main kLex README](../../README.md) for language documentation.
