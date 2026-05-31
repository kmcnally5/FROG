package eval

// WorkerTransportSpawn, when set, is called by the bridge dispatcher for
// `bridgeOpen({"kind": "worker", ...})`. The WASM build wires this up at
// init() time (see eval/bridge_worker_wasm.go); desktop leaves it nil
// and `kind: "worker"` returns BRIDGE_TRANSPORT_UNAVAILABLE.
//
// Set once at init(). The argument is the transport hash; return an
// Object — either a (*Bridge, NULL) success tuple or a (NULL, *Error)
// failure tuple — matching the bridgeOpen contract.
//
// Same hook pattern as eval.ExternalCallable and
// eval.EmbeddedImportLookup — set at init, read on the hot path.
var WorkerTransportSpawn func(transport *Hash) Object
