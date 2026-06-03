package eval

import (
	"fmt"
	"io"
	"klex/ast"
	"net"
	"strconv"
)

func init() {
	// tcpDial — open a TCP connection to host:port.
	//
	// The client side of the networking API. Follows the standard two-path
	// pattern: on success the first tuple element is a connection and the second
	// is null; on failure the first is null and the second is an error. Read and
	// write with netRead/netWrite, then netClose.
	//
	// @sig     tcpDial(host: string, port: int) -> (connection, error)
	// @param   host  the hostname or IP to connect to
	// @param   port  the TCP port
	// @returns a (connection, null) tuple on success, or (null, error) on failure
	// @errors  TypeError if host isn't a string or port isn't an integer; returns a connection error in the tuple's second slot if the dial fails
	// @example no-run conn, err = tcpDial("example.com", 80)
	// @since   0.1.0
	// @see     tcpListen, netRead, netWrite, netClose
	Builtins["tcpDial"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("tcpDial expects 2 arguments (host, port)", ast.Pos{})
		}
		host, ok1 := args[0].(*String)
		port, ok2 := args[1].(*Integer)
		if !ok1 {
			return typeError(fmt.Sprintf("tcpDial: host must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if !ok2 {
			return typeError(fmt.Sprintf("tcpDial: port must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		addr := host.Value + ":" + strconv.Itoa(port.Value)
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, runtimeError("tcpDial: "+err.Error(), ast.Pos{})}}
		}
		return &Tuple{Elements: []Object{&NetConn{Conn: conn}, NULL}}
	}}

	// tcpListen — start a TCP server, yielding accepted connections on a channel.
	//
	// The server side. Returns a channel that delivers one connection per client;
	// iterate it with for-in to accept, and `break` out of the loop (which cancels
	// the channel) to stop the server and release the port. Spawn an async task per
	// connection to handle many clients concurrently. Unlike the dial/read/write
	// builtins it returns the channel directly, not a tuple.
	//
	// @sig     tcpListen(host: string, port: int) -> channel
	// @param   host  the interface to bind (e.g. "0.0.0.0" for all)
	// @param   port  the TCP port to listen on
	// @returns a channel of incoming connections
	// @errors  TypeError if host isn't a string or port isn't an integer; RuntimeError if the port can't be bound
	// @example no-run for conn in tcpListen("0.0.0.0", 8080) { async(fn() { handle(conn) }) }
	// @since   0.1.0
	// @see     tcpDial, netRead, netWrite, netClose
	Builtins["tcpListen"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("tcpListen expects 2 arguments (host, port)", ast.Pos{})
		}
		host, ok1 := args[0].(*String)
		port, ok2 := args[1].(*Integer)
		if !ok1 {
			return typeError(fmt.Sprintf("tcpListen: host must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if !ok2 {
			return typeError(fmt.Sprintf("tcpListen: port must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		addr := host.Value + ":" + strconv.Itoa(port.Value)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return runtimeError("tcpListen: "+err.Error(), ast.Pos{})
		}

		ch := &Channel{
			ch:   make(chan Object, 8),
			done: make(chan struct{}),
		}

		// Watcher: close the listener when the consumer cancels the channel.
		go func() {
			<-ch.done
			ln.Close()
		}()

		// Producer: accept connections and send them to the channel.
		go func() {
			defer close(ch.ch)
			for {
				conn, err := ln.Accept()
				if err != nil {
					return // listener was closed or real error
				}
				select {
				case ch.ch <- &NetConn{Conn: conn}:
				case <-ch.done:
					conn.Close()
					return
				}
			}
		}()

		return ch
	}}

	// netRead — read up to maxBytes from a connection.
	//
	// Returns whatever bytes are available, up to maxBytes — a single read isn't
	// guaranteed to fill the buffer, so loop until you have a full message. EOF is
	// not an error: at end of stream you get an empty string and a null error.
	//
	// @sig     netRead(conn: connection, maxBytes: int) -> (string, error)
	// @param   conn      a connection from tcpDial or tcpListen
	// @param   maxBytes  the maximum number of bytes to read (must be positive)
	// @returns a (data, null) tuple on success (data is "" at EOF), or (null, error) on failure
	// @errors  TypeError if conn isn't a connection or maxBytes isn't an integer; RuntimeError if maxBytes <= 0; returns a read error in the tuple's second slot
	// @example no-run data, err = netRead(conn, 4096)
	// @since   0.1.0
	// @see     netWrite, tcpDial, netClose
	Builtins["netRead"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("netRead expects 2 arguments (conn, maxBytes)", ast.Pos{})
		}
		nc, ok1 := args[0].(*NetConn)
		maxBytes, ok2 := args[1].(*Integer)
		if !ok1 {
			return typeError(fmt.Sprintf("netRead: first argument must be conn, got %s", args[0].Type()), ast.Pos{})
		}
		if !ok2 {
			return typeError(fmt.Sprintf("netRead: maxBytes must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		if maxBytes.Value <= 0 {
			return runtimeError("netRead: maxBytes must be positive", ast.Pos{})
		}
		buf := make([]byte, maxBytes.Value)
		n, err := nc.Conn.Read(buf)
		if err != nil && err != io.EOF {
			return &Tuple{Elements: []Object{NULL, runtimeError("netRead: "+err.Error(), ast.Pos{})}}
		}
		return &Tuple{Elements: []Object{&String{Value: string(buf[:n])}, NULL}}
	}}

	// netWrite — send data over a connection.
	//
	// Writes the whole string before returning. On success both tuple elements
	// are null (there's no useful value to return); on failure the second element
	// is the error.
	//
	// @sig     netWrite(conn: connection, data: string) -> (null, error)
	// @param   conn  a connection from tcpDial or tcpListen
	// @param   data  the string to send
	// @returns (null, null) on success, or (null, error) on failure
	// @errors  TypeError if conn isn't a connection or data isn't a string; returns a write error in the tuple's second slot
	// @example no-run _, err = netWrite(conn, "hello\n")
	// @since   0.1.0
	// @see     netRead, tcpDial, netClose
	Builtins["netWrite"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("netWrite expects 2 arguments (conn, data)", ast.Pos{})
		}
		nc, ok1 := args[0].(*NetConn)
		data, ok2 := args[1].(*String)
		if !ok1 {
			return typeError(fmt.Sprintf("netWrite: first argument must be conn, got %s", args[0].Type()), ast.Pos{})
		}
		if !ok2 {
			return typeError(fmt.Sprintf("netWrite: data must be string, got %s", args[1].Type()), ast.Pos{})
		}
		_, err := nc.Conn.Write([]byte(data.Value))
		if err != nil {
			return &Tuple{Elements: []Object{NULL, runtimeError("netWrite: "+err.Error(), ast.Pos{})}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// netClose — close a connection and release its resources.
	//
	// Idempotent: closing an already-closed connection is a no-op. Always close
	// connections you open so file descriptors aren't leaked.
	//
	// @sig     netClose(conn: connection) -> null
	// @param   conn  the connection to close
	// @returns null
	// @errors  TypeError if conn isn't a connection
	// @example no-run netClose(conn)
	// @since   0.1.0
	// @see     tcpDial, netRead, netWrite
	Builtins["netClose"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("netClose expects 1 argument", ast.Pos{})
		}
		nc, ok := args[0].(*NetConn)
		if !ok {
			return typeError(fmt.Sprintf("netClose: argument must be conn, got %s", args[0].Type()), ast.Pos{})
		}
		if nc.Conn != nil {
			nc.Conn.Close()
			nc.Conn = nil
		}
		return NULL
	}}

	// dnsLookup — resolve a hostname to its IP addresses.
	//
	// Returns every address the resolver finds (both IPv4 and IPv6) as an array
	// of strings. Follows the two-path pattern: (addresses, null) on success or
	// (null, error) on failure.
	//
	// @sig     dnsLookup(hostname: string) -> (array, error)
	// @param   hostname  the name to resolve
	// @returns an (addresses, null) tuple on success, or (null, error) on failure
	// @errors  TypeError if hostname isn't a string; returns a resolver error in the tuple's second slot if lookup fails
	// @example no-run addrs, err = dnsLookup("example.com")
	// @since   0.1.0
	// @see     tcpDial, tcpListen
	Builtins["dnsLookup"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("dnsLookup expects 1 argument", ast.Pos{})
		}
		host, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("dnsLookup: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		addrs, err := net.LookupHost(host.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, runtimeError("dnsLookup: "+err.Error(), ast.Pos{})}}
		}
		elements := make([]Object, len(addrs))
		for i, addr := range addrs {
			elements[i] = &String{Value: addr}
		}
		return &Tuple{Elements: []Object{&Array{Elements: elements}, NULL}}
	}}
}
