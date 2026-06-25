// Command vaultnode runs a self-hosted storage point. Run two or more of these
// (on localhost, your own servers, anywhere you control) and point vaultctl at
// them with --node. Nodes only ever hold encrypted, content-addressed blocks.
//
//	vaultnode --listen :8443 --data ./node-data --token "$NFTVAULT_NODE_TOKEN"
//	vaultnode --listen :8443 --data ./node-data --tls-cert c.pem --tls-key k.pem
//
// The token may also be supplied via NFTVAULT_NODE_TOKEN to keep it out of the
// process list. With no token and no TLS, bind to localhost only.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"nftvault/cas"
	"nftvault/node"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8443", "address to listen on")
	dataDir := flag.String("data", "node-data", "directory for stored blocks")
	name := flag.String("name", "node", "backend name (for logs)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file (recommended)")
	tlsKey := flag.String("tls-key", "", "TLS key file")
	flag.Parse()

	token := os.Getenv("NFTVAULT_NODE_TOKEN")

	be, err := cas.NewFSBackend(*name, *dataDir)
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}
	srv := &http.Server{Addr: *listen, Handler: node.NewServer(be, token).Handler()}

	scheme := "http"
	if *tlsCert != "" && *tlsKey != "" {
		scheme = "https"
	}
	authNote := "auth: ON"
	if token == "" {
		authNote = "auth: OFF (set NFTVAULT_NODE_TOKEN; bind localhost only)"
	}
	log.Printf("vaultnode %q listening on %s://%s  data=%s  %s", *name, scheme, *listen, *dataDir, authNote)

	if scheme == "https" {
		log.Fatal(srv.ListenAndServeTLS(*tlsCert, *tlsKey))
	}
	log.Fatal(srv.ListenAndServe())
}
