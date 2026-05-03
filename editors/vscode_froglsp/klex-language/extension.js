'use strict'

const vscode = require('vscode')
const path = require('path')
const fs = require('fs')
const { LanguageClient, LanguageClientOptions } = require('vscode-languageclient/node')

let client = null

// Find the froglsp binary in common locations
function findFrogLspBinary() {
	// Try workspace root + froglsp
	const workspaceFolder = vscode.workspace.workspaceFolders?.[0]?.uri?.fsPath
	if (workspaceFolder) {
		const binaryPath = path.join(workspaceFolder, 'froglsp')
		if (fs.existsSync(binaryPath)) {
			return binaryPath
		}
	}

	// Try current directory + froglsp
	const cwd = process.cwd()
	const localBinary = path.join(cwd, 'froglsp')
	if (fs.existsSync(localBinary)) {
		return localBinary
	}

	// If in the kLex project directory, try to build/run
	if (workspaceFolder && workspaceFolder.includes('kLex')) {
		const snowballBinary = path.join(workspaceFolder, 'froglsp')
		if (fs.existsSync(snowballBinary)) {
			return snowballBinary
		}
		// Try to use go run if in project root
		if (fs.existsSync(path.join(workspaceFolder, 'snowball', 'froglsp'))) {
			return 'go' // Special marker to use go run
		}
	}

	// Default: try "froglsp" on PATH
	return 'froglsp'
}

// Create server options that will launch the LSP server
function createServerOptions() {
	const command = findFrogLspBinary()
	const workspaceFolder = vscode.workspace.workspaceFolders?.[0]?.uri?.fsPath

	if (command === 'go') {
		// Use go run if we found the source but not the binary
		return {
			command: 'go',
			args: ['run', './snowball/froglsp/'],
			options: { cwd: workspaceFolder }
		}
	}

	return {
		command: command,
		args: [],
		options: { cwd: workspaceFolder }
	}
}

async function activate(context) {
	console.log('=== kLex LSP Extension Activating ===')
	try {
		const serverOptions = createServerOptions()
		console.log('Server options:', JSON.stringify(serverOptions, null, 2))

		// Create output channel for LSP logging
		const outputChannel = vscode.window.createOutputChannel('kLex Language Server')

		const clientOptions = {
			documentSelector: [{ scheme: 'file', language: 'klex' }],
			synchronization: {
				textDocument: {
					didSave: true,
					didOpen: true,
					didChange: true
				}
			},
			outputChannel: outputChannel,
			revealOutputChannelOn: 'warn' // Show output on warnings
		}

		client = new LanguageClient(
			'klex-lsp',
			'kLex Language Server',
			serverOptions,
			clientOptions
		)

		// Start the client
		console.log('Starting LSP client...')
		await client.start()
		context.subscriptions.push(client)

		console.log('=== kLex Language Server started successfully ===')

		// Manually notify server about already-open .lex documents
		console.log('Notifying server about open documents...')
		for (const editor of vscode.window.visibleTextEditors) {
			if (editor.document.languageId === 'klex') {
				console.log('Sending didOpen for:', editor.document.uri.toString())
				await client.sendNotification('textDocument/didOpen', {
					textDocument: {
						uri: editor.document.uri.toString(),
						languageId: 'klex',
						version: 1,
						text: editor.document.getText()
					}
				})
			}
		}

		// Listen for future opens
		context.subscriptions.push(
			vscode.workspace.onDidOpenTextDocument((doc) => {
				if (doc.languageId === 'klex') {
					console.log('New document opened:', doc.uri.toString())
					client.sendNotification('textDocument/didOpen', {
						textDocument: {
							uri: doc.uri.toString(),
							languageId: 'klex',
							version: doc.version,
							text: doc.getText()
						}
					})
				}
			})
		)
	} catch (error) {
		console.error('Failed to start kLex Language Server:', error)
		console.error('Full error details:', error.stack)
		vscode.window.showErrorMessage(
			'Failed to start kLex Language Server: ' + (error.message || String(error)) +
			'\n\nPlease check the Output panel (View > Output) for details.'
		)
	}
}

function deactivate() {
	if (client) {
		return client.stop()
	}
}

module.exports = { activate, deactivate }
