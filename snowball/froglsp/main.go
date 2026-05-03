package main

import (
	"os"
)

func main() {
	// Create transport over stdin/stdout
	transport := NewTransport(os.Stdin, os.Stdout)

	// Create server
	server := NewServer(transport)

	// Run the server
	if err := server.Run(); err != nil {
		LogMessage("server error: %v", err)
		os.Exit(1)
	}
}
