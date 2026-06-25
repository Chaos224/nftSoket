// Package node implements a self-hosted storage point: a small HTTP daemon that
// serves the cas.Backend interface over the network, plus a client
// (RemoteBackend) that lets a vault treat a remote node as one of its storage
// points. You run the nodes yourself (localhost, your own servers, a friend's
// machine) — nothing depends on a third-party service.
//
// Trust model: nodes only ever see encrypted, content-addressed blocks. They
// cannot read your data; a bearer token only governs who may store/serve blocks.
// Because blocks are content-addressed, a node also cannot tamper undetected:
// both the node (on write) and the client (on read) verify address == hash.
package node

// Wire protocol, versioned so future changes never break existing nodes/clients.
const (
	// APIVersion is sent and required on every request via the header below.
	APIVersion = "1"
	apiHeader  = "X-Vault-Api"
	authHeader = "Authorization"
	authScheme = "Bearer "

	// Route prefix; {addr} is a url-escaped content address ("v1:<hex>").
	blockPath  = "/v1/block/"
	healthPath = "/v1/healthz"
)
