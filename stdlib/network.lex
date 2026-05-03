// stdlib/network.lex — TCP networking and DNS for kLex
//
// Wraps the tcpDial, tcpListen, netRead, netWrite, netClose, dnsLookup
// builtins into a consistent, named API.
//
// All operations return (result, null) on success and (null, error) on
// failure — the standard kLex two-path error pattern.
//
// Usage:
//   import "network.lex" as net
//
//   // TCP client
//   let conn, err = net.dial("example.com", 80)
//   if err != null { println("connect failed:", err.message)  return }
//   net.write(conn, "GET / HTTP/1.0\r\nHost: example.com\r\n\r\n")
//   let response, err = net.readAll(conn)
//   net.close(conn)
//
//   // TCP server
//   net.tcpServer("0.0.0.0", 8080, fn(conn) {
//       let data, err = net.read(conn, 4096)
//       net.write(conn, "HTTP/1.0 200 OK\r\n\r\nHello from kLex!\n")
//       net.close(conn)
//   })


// ----------------------------------------------------------------------------
// Core connection API
// ----------------------------------------------------------------------------

// dial opens a TCP connection to host:port.
// Returns (conn, null) on success, (null, error) on failure.
fn dial(host, port) {
    return tcpDial(host, port)
}

// listen starts a TCP server on host:port.
// Returns a Channel of connections — use with for-in to accept each one.
// Break out of the loop to stop the server.
//
//   for conn in net.listen("0.0.0.0", 8080) {
//       let c = conn
//       async(fn() { handleConn(c) })
//   }
fn listen(host, port) {
    return tcpListen(host, port)
}

// read reads up to maxBytes from conn.
// Returns (string, null) on success, (null, error) on failure or closed conn.
fn read(conn, maxBytes) {
    return netRead(conn, maxBytes)
}

// write sends data to conn.
// Returns (null, null) on success, (null, error) on failure.
fn write(conn, data) {
    return netWrite(conn, data)
}

// close closes a connection.
fn close(conn) {
    return netClose(conn)
}

// lookup resolves a hostname to an array of IP address strings.
// Returns (addresses, null) on success, (null, error) on failure.
fn lookup(host) {
    return dnsLookup(host)
}


// ----------------------------------------------------------------------------
// Higher-level helpers
// ----------------------------------------------------------------------------

// readAll reads from conn until EOF, accumulating all received data.
// Returns (string, null) on success, (null, error) on read error.
// Suitable for short responses — for streaming data use read() in a loop.
fn readAll(conn) {
    let result = ""
    while true {
        chunk, err = netRead(conn, 4096)
        if err != null { return null, err }
        if chunk == "" { break }
        result = result + chunk
    }
    return result, null
}

// writeLine writes data followed by \n to conn.
// Returns (null, null) on success, (null, error) on failure.
fn writeLine(conn, data) {
    return netWrite(conn, data + "\n")
}

// tcpServer starts a TCP server and calls handlerFn(conn) in a new goroutine
// for each incoming connection. Blocks until the server is cancelled.
// handlerFn is responsible for closing the connection when done.
//
//   net.tcpServer("0.0.0.0", 8080, fn(conn) {
//       let data, err = net.read(conn, 1024)
//       net.write(conn, "echo: " + data)
//       net.close(conn)
//   })
fn tcpServer(host, port, handlerFn) {
    for conn in tcpListen(host, port) {
        let c = conn
        async(fn() { handlerFn(c) })
    }
}
