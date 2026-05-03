package eval

import (
	"fmt"
	"io"
	"klex/ast"
	"net"
	"strconv"
)

func init() {
	// tcpDial opens a TCP connection to host:port.
	// Returns (conn, null) on success, (null, error) on failure.
	// Usage: conn, err = tcpDial("example.com", 80)

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

	// tcpListen starts a TCP server on host:port and returns a Channel of NetConn.
	// Each accepted connection is sent as a NetConn value.
	// Cancelling the channel (break in a for-in loop) stops the server.
	// Usage: for conn in tcpListen("0.0.0.0", 8080) { async(fn() { handle(conn) }) }
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

	// netRead reads up to maxBytes from conn.
	// Returns (string, null) on success or EOF, (null, error) on failure.
	// Usage: data, err = netRead(conn, 4096)
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

	// netWrite sends data to conn.
	// Returns (null, null) on success, (null, error) on failure.
	// Usage: _, err = netWrite(conn, "hello\n")
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

	// netClose closes a NetConn.
	// Usage: netClose(conn)
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

	// dnsLookup resolves a hostname to an array of IP address strings.
	// Returns (addresses, null) on success, (null, error) on failure.
	// Usage: addrs, err = dnsLookup("example.com")
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
