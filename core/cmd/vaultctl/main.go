// Command vaultctl is the reference CLI for the NFT Vault security core. It
// demonstrates the full flow end to end: generate a paper access key, split it
// across custodians, and encrypt/store/retrieve files redundantly.
//
// Secrets are read from stdin (no echo for the passphrase) or environment
// variables, never from command-line flags, so they do not leak into shell
// history or the process list.
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/term"

	"nftvault/cas"
	"nftvault/node"
	"nftvault/paperkey"
	"nftvault/shamir"
	"nftvault/vault"
)

// storageDesc summarizes the configured storage points for display.
func storageDesc(args []string) string {
	nodes := flagVals(args, "--node")
	if env := os.Getenv("NFTVAULT_NODES"); env != "" {
		nodes = append(nodes, strings.Split(env, ",")...)
	}
	if len(nodes) > 0 {
		return strings.Join(nodes, ", ")
	}
	return flagVal(args, "--vault", "vault-data")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = cmdKeygen(os.Args[2:])
	case "restore":
		err = cmdRestore(os.Args[2:])
	case "split":
		err = cmdSplit(os.Args[2:])
	case "combine":
		err = cmdCombine(os.Args[2:])
	case "put":
		err = cmdPut(os.Args[2:])
	case "get":
		err = cmdGet(os.Args[2:])
	case "health":
		err = cmdHealth(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`nft vaultctl — secure decentralized vault (reference CLI)

  keygen  [--words 12|24]        generate a paper access key + printable sheet
  restore                        validate a written mnemonic, show fingerprint
  split   --shares N --threshold K   split the master key into Shamir shares
  combine                        recombine shares into the master key (hex)
  put     <file> [storage] [--name NAME]   encrypt + store redundantly
  get     <root> [storage] --out FILE       retrieve + decrypt
  health  <root> [storage]    show which storage points hold the data

Secrets: set NFTVAULT_MNEMONIC / NFTVAULT_PASSPHRASE to avoid prompts.
Storage [storage]: either
  --node URL   (repeatable, >=2) self-hosted vaultnode points; token via
               NFTVAULT_NODE_TOKEN. Or set NFTVAULT_NODES=url1,url2.
  --vault DIR  two local directories DIR/a and DIR/b (default ./vault-data).
`)
}

// --- helpers ---------------------------------------------------------------

func flagVal(args []string, name, def string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return def
}

// flagVals returns every value of a repeatable flag (e.g. multiple --node).
func flagVals(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			out = append(out, args[i+1])
		}
	}
	return out
}

func positional(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			i++ // skip its value
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func readMnemonic() (string, error) {
	if m := os.Getenv("NFTVAULT_MNEMONIC"); m != "" {
		return m, nil
	}
	fmt.Fprint(os.Stderr, "Enter access key (mnemonic words): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func readPassphrase() (string, error) {
	if p, ok := os.LookupEnv("NFTVAULT_PASSPHRASE"); ok {
		return p, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", nil // non-interactive and unset: treat as empty
	}
	fmt.Fprint(os.Stderr, "Enter passphrase (empty for none): ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}

func unlockedKey() (*paperkey.AccessKey, error) {
	m, err := readMnemonic()
	if err != nil {
		return nil, err
	}
	ak, err := paperkey.Restore(m)
	if err != nil {
		return nil, err
	}
	pass, err := readPassphrase()
	if err != nil {
		return nil, err
	}
	if err := ak.Unlock(pass); err != nil {
		return nil, err
	}
	return ak, nil
}

// buildStore assembles the redundant store from the storage points requested on
// the command line: one or more remote nodes (--node URL, repeatable, or the
// NFTVAULT_NODES env var), otherwise two local directories under --vault DIR.
func buildStore(args []string) (*cas.Store, error) {
	nodes := flagVals(args, "--node")
	if env := os.Getenv("NFTVAULT_NODES"); env != "" {
		nodes = append(nodes, strings.Split(env, ",")...)
	}
	if len(nodes) > 0 {
		token := os.Getenv("NFTVAULT_NODE_TOKEN")
		backends := make([]cas.Backend, 0, len(nodes))
		for i, u := range nodes {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			backends = append(backends, node.NewRemoteBackend(fmt.Sprintf("point-%d", i+1), u, token))
		}
		return cas.NewStore(backends...)
	}
	vaultDir := flagVal(args, "--vault", "vault-data")
	a, err := cas.NewFSBackend("point-a", filepath.Join(vaultDir, "a"))
	if err != nil {
		return nil, err
	}
	b, err := cas.NewFSBackend("point-b", filepath.Join(vaultDir, "b"))
	if err != nil {
		return nil, err
	}
	return cas.NewStore(a, b)
}

// --- commands --------------------------------------------------------------

func cmdKeygen(args []string) error {
	strength := paperkey.Words24
	if flagVal(args, "--words", "24") == "12" {
		strength = paperkey.Words12
	}
	ak, err := paperkey.Generate(strength)
	if err != nil {
		return err
	}
	pass, err := readPassphrase()
	if err != nil {
		return err
	}
	if err := ak.Unlock(pass); err != nil {
		return err
	}
	sheet, err := ak.RenderSheet()
	if err != nil {
		return err
	}
	fp, _ := ak.Fingerprint()
	fmt.Print(sheet)
	fmt.Printf("\nVault fingerprint: %s\n", fp)
	fmt.Fprintln(os.Stderr, "\n*** Write these words down and store them offline. They are shown ONCE. ***")
	return nil
}

func cmdRestore(args []string) error {
	ak, err := unlockedKey()
	if err != nil {
		return err
	}
	fp, _ := ak.Fingerprint()
	fmt.Printf("OK. Vault fingerprint: %s\n", fp)
	return nil
}

func cmdSplit(args []string) error {
	shares, _ := strconv.Atoi(flagVal(args, "--shares", "3"))
	threshold, _ := strconv.Atoi(flagVal(args, "--threshold", "2"))
	ak, err := unlockedKey()
	if err != nil {
		return err
	}
	master, err := ak.MasterKey()
	if err != nil {
		return err
	}
	parts, err := shamir.Split(master, shares, threshold)
	if err != nil {
		return err
	}
	fmt.Printf("# Master key split into %d shares; any %d reconstruct it.\n", shares, threshold)
	fmt.Println("# Give each share to a different custodian / location.")
	for _, p := range parts {
		fmt.Printf("%02x:%s\n", p.X, hex.EncodeToString(p.Y))
	}
	return nil
}

func cmdCombine(args []string) error {
	fmt.Fprintln(os.Stderr, "Paste shares (one per line, 'XX:hex'), end with EOF (Ctrl-D):")
	var parts []shamir.Share
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		x, y, err := parseShare(line)
		if err != nil {
			return err
		}
		parts = append(parts, shamir.Share{X: x, Y: y})
	}
	master, err := shamir.Combine(parts)
	if err != nil {
		return err
	}
	fmt.Printf("master-key: %s\n", hex.EncodeToString(master))
	return nil
}

func parseShare(line string) (byte, []byte, error) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return 0, nil, fmt.Errorf("bad share (want XX:hex): %q", line)
	}
	xb, err := hex.DecodeString(line[:i])
	if err != nil || len(xb) != 1 {
		return 0, nil, fmt.Errorf("bad share x: %q", line[:i])
	}
	y, err := hex.DecodeString(line[i+1:])
	if err != nil {
		return 0, nil, fmt.Errorf("bad share y: %w", err)
	}
	return xb[0], y, nil
}

func cmdPut(args []string) error {
	pos := positional(args)
	if len(pos) < 1 {
		return fmt.Errorf("usage: put <file> [--vault DIR] [--name NAME]")
	}
	src := pos[0]
	name := flagVal(args, "--name", filepath.Base(src))

	ak, err := unlockedKey()
	if err != nil {
		return err
	}
	store, err := buildStore(args)
	if err != nil {
		return err
	}
	v, err := vault.Open(ak, store)
	if err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	root, err := v.PutFile(name, f)
	if err != nil {
		return err
	}
	fmt.Printf("stored %q\n  points      : %s\n  fingerprint : %s\n  root        : %s\n", name, storageDesc(args), v.Fingerprint(), root)
	return nil
}

func cmdGet(args []string) error {
	pos := positional(args)
	if len(pos) < 1 {
		return fmt.Errorf("usage: get <root> --out FILE [--vault DIR]")
	}
	root := pos[0]
	out := flagVal(args, "--out", "")
	if out == "" {
		return fmt.Errorf("--out FILE is required")
	}

	ak, err := unlockedKey()
	if err != nil {
		return err
	}
	store, err := buildStore(args)
	if err != nil {
		return err
	}
	v, err := vault.Open(ak, store)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	m, err := v.GetFile(root, f)
	if err != nil {
		return err
	}
	fmt.Printf("retrieved %q (%d bytes, %d chunks) -> %s\n", m.Name, m.Size, len(m.Chunks), out)
	return nil
}

func cmdHealth(args []string) error {
	pos := positional(args)
	if len(pos) < 1 {
		return fmt.Errorf("usage: health <root> [--vault DIR]")
	}
	root := pos[0]
	store, err := buildStore(args)
	if err != nil {
		return err
	}
	fmt.Printf("storage points for %s:\n", root)
	for name, ok := range store.Health(root) {
		status := "MISSING"
		if ok {
			status = "present"
		}
		fmt.Printf("  %-10s %s\n", name, status)
	}
	return nil
}
