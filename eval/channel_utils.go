package eval

// closeChannelDone idempotently closes a Channel's done signal. Both the
// consumer (eval.go's for-in break handler) and the bridge reader (on natural
// stream end) may want to close it; whoever loses the race recovers from the
// "close of closed channel" panic.
//
// Lives in its own file with no build tag so it's available to every
// platform — Metal kernels (darwin), MTL stubs (linux/wasm/windows), and
// bridge readers (non-js) all share this helper.
func closeChannelDone(ch *Channel) {
	defer func() { recover() }()
	close(ch.done)
}
