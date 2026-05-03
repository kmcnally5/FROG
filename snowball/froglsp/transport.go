package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Transport handles JSON-RPC 2.0 over stdin/stdout with Content-Length framing
type Transport struct {
	reader    *bufio.Reader
	writer    *bufio.Writer
	mu        sync.Mutex
	requestID int
}

func NewTransport(stdin io.Reader, stdout io.Writer) *Transport {
	return &Transport{
		reader: bufio.NewReader(stdin),
		writer: bufio.NewWriter(stdout),
	}
}

// Message is the union of request, response, and notification
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// ReadMessage reads the next message from stdin
func (t *Transport) ReadMessage() (*Message, error) {
	headers := make(map[string]string)

	// Read headers (Content-Length, Content-Type, etc.)
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}

		// Remove trailing \r\n or \n
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // empty line signals end of headers
		}

		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			headers[parts[0]] = parts[1]
		}

		if err == io.EOF {
			return nil, io.EOF
		}
	}

	contentLengthStr, ok := headers["Content-Length"]
	if !ok {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	contentLength, err := strconv.Atoi(contentLengthStr)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Length: %w", err)
	}

	// Read exactly contentLength bytes
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.reader, body); err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// SendResponse sends a response message
func (t *Transport) SendResponse(id interface{}, result interface{}, err *RPCError) error {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
	}
	if id != nil {
		msg["id"] = id
	}
	if err != nil {
		msg["error"] = err
	} else {
		msg["result"] = result
	}
	return t.sendJSON(msg)
}

// SendNotification sends a notification message
func (t *Transport) SendNotification(method string, params interface{}) error {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	return t.sendJSON(msg)
}

// sendJSON encodes and writes a JSON message with Content-Length framing
func (t *Transport) sendJSON(msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := t.writer.WriteString(header); err != nil {
		return err
	}
	if _, err := t.writer.Write(body); err != nil {
		return err
	}
	return t.writer.Flush()
}

// LogMessage writes a message to stderr (for debugging)
func LogMessage(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[LSP] "+format+"\n", args...)
}
