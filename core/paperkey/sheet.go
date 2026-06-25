package paperkey

import (
	"fmt"
	"strings"
)

// RenderSheet produces a printable, plain-text "paper key" sheet, the kind a
// user prints and stores in a safe — like a cryptocurrency paper wallet. It
// shows the numbered words, the vault fingerprint, and recovery instructions.
// It never includes the seed, master key, or passphrase.
func (k *AccessKey) RenderSheet() (string, error) {
	words := strings.Fields(k.Mnemonic)
	fp := "(locked — unlock to show)"
	if len(k.seed) > 0 {
		if f, err := k.Fingerprint(); err == nil {
			fp = f
		}
	}

	var b strings.Builder
	const width = 64
	line := strings.Repeat("=", width)
	b.WriteString(line + "\n")
	b.WriteString(center("NFT VAULT — PAPER ACCESS KEY", width) + "\n")
	b.WriteString(center("keep offline · never share · anyone with this controls the vault", width) + "\n")
	b.WriteString(line + "\n\n")

	// Words in two columns, numbered.
	half := (len(words) + 1) / 2
	for i := 0; i < half; i++ {
		left := fmt.Sprintf("%2d. %-14s", i+1, words[i])
		right := ""
		if j := i + half; j < len(words) {
			right = fmt.Sprintf("%2d. %-14s", j+1, words[j])
		}
		b.WriteString("    " + left + "   " + right + "\n")
	}

	b.WriteString("\n" + strings.Repeat("-", width) + "\n")
	b.WriteString(fmt.Sprintf(" Vault fingerprint : %s\n", fp))
	b.WriteString(fmt.Sprintf(" Words             : %d\n", len(words)))
	b.WriteString(" Passphrase (25th) : ____________________  (optional, memorize)\n")
	b.WriteString(strings.Repeat("-", width) + "\n\n")
	b.WriteString(" RECOVERY: enter these words in order to restore access on any\n")
	b.WriteString(" device. The order matters. If you set a passphrase you must also\n")
	b.WriteString(" enter it — it is NOT written here on purpose.\n")
	b.WriteString(line + "\n")
	return b.String(), nil
}

func center(s string, w int) string {
	if len(s) >= w {
		return s
	}
	pad := (w - len(s)) / 2
	return strings.Repeat(" ", pad) + s
}
