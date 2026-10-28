package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// Release metadata is signed with OpenSSH's file signatures (SSHSIG, the
// format of `ssh-keygen -Y sign`), using an Ed25519 key. The same signature
// is verified by this package, by scripts/install.sh and by install.ps1
// (with `ssh-keygen -Y verify`), so one trust anchor serves every path.

// SignatureNamespace binds signatures to TuiSense releases; a signature made
// for another purpose with the same key does not verify.
const SignatureNamespace = "tuisense-release"

var errSig = errors.New("invalid release signature")

// TrustedKey is a pinned Ed25519 public key.
type TrustedKey struct {
	Name string
	Key  ed25519.PublicKey
}

// ParseAuthorizedKey parses an "ssh-ed25519 AAAA… comment" line (an
// allowed_signers line "principal [options] ssh-ed25519 AAAA…" is also accepted).
func ParseAuthorizedKey(line string) (TrustedKey, error) {
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "ssh-ed25519" {
			continue
		}
		blob, err := base64.StdEncoding.DecodeString(fields[i+1])
		if err != nil {
			return TrustedKey{}, err
		}
		r := wire{b: blob}
		if r.str() != "ssh-ed25519" {
			return TrustedKey{}, errors.New("not an ssh-ed25519 key")
		}
		k := r.bytes()
		if r.err || len(k) != ed25519.PublicKeySize {
			return TrustedKey{}, errors.New("malformed ssh-ed25519 key")
		}
		name := ""
		if i > 0 {
			name = fields[0]
		} else if len(fields) > 2 {
			name = fields[2]
		}
		return TrustedKey{Name: name, Key: ed25519.PublicKey(k)}, nil
	}
	return TrustedKey{}, errors.New("no ssh-ed25519 key found")
}

// ParseTrustedKeys parses one key per line, ignoring blanks and # comments.
func ParseTrustedKeys(text string) ([]TrustedKey, error) {
	var out []TrustedKey
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, err := ParseAuthorizedKey(line)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

// VerifySSHSig checks an armored SSHSIG signature over message against any
// of the trusted keys, for the TuiSense namespace.
func VerifySSHSig(message, armored []byte, keys []TrustedKey) (TrustedKey, error) {
	if len(keys) == 0 {
		return TrustedKey{}, ErrNoTrustedKey
	}
	blob, err := dearmor(armored)
	if err != nil {
		return TrustedKey{}, err
	}
	r := wire{b: blob}
	if string(r.raw(6)) != "SSHSIG" || r.u32() != 1 {
		return TrustedKey{}, errSig
	}
	pub := r.bytes()
	ns := r.str()
	_ = r.bytes() // reserved
	hashAlg := r.str()
	sigBlob := r.bytes()
	if r.err || ns != SignatureNamespace || hashAlg != "sha512" {
		return TrustedKey{}, fmt.Errorf("%w (namespace %q, hash %q)", errSig, ns, hashAlg)
	}
	pr := wire{b: pub}
	if pr.str() != "ssh-ed25519" {
		return TrustedKey{}, errSig
	}
	pk := pr.bytes()
	sr := wire{b: sigBlob}
	if sr.str() != "ssh-ed25519" {
		return TrustedKey{}, errSig
	}
	sig := sr.bytes()
	if pr.err || sr.err || len(pk) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return TrustedKey{}, errSig
	}
	signed := signedData(message)
	for _, k := range keys {
		if bytes.Equal(k.Key, pk) && ed25519.Verify(k.Key, signed, sig) {
			return k, nil
		}
	}
	return TrustedKey{}, ErrUntrustedSignature
}

// SignSSHSig produces an armored SSHSIG signature (used by the release tool
// and tests; CI may equally use `ssh-keygen -Y sign -n tuisense-release`).
func SignSSHSig(message []byte, priv ed25519.PrivateKey) []byte {
	pub := priv.Public().(ed25519.PublicKey)
	pubBlob := wireBytes(wireString("ssh-ed25519"), wireString(string(pub)))
	sig := ed25519.Sign(priv, signedData(message))
	sigBlob := wireBytes(wireString("ssh-ed25519"), wireString(string(sig)))
	var b bytes.Buffer
	b.WriteString("SSHSIG")
	_ = binary.Write(&b, binary.BigEndian, uint32(1))
	b.Write(wireString(string(pubBlob)))
	b.Write(wireString(SignatureNamespace))
	b.Write(wireString(""))
	b.Write(wireString("sha512"))
	b.Write(wireString(string(sigBlob)))
	enc := base64.StdEncoding.EncodeToString(b.Bytes())
	var out strings.Builder
	out.WriteString("-----BEGIN SSH SIGNATURE-----\n")
	for len(enc) > 70 {
		out.WriteString(enc[:70] + "\n")
		enc = enc[70:]
	}
	out.WriteString(enc + "\n-----END SSH SIGNATURE-----\n")
	return []byte(out.String())
}

// AuthorizedKey formats a public key as an "ssh-ed25519 AAAA… name" line.
func AuthorizedKey(pub ed25519.PublicKey, name string) string {
	blob := wireBytes(wireString("ssh-ed25519"), wireString(string(pub)))
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " " + name
}

func signedData(message []byte) []byte {
	h := sha512.Sum512(message)
	var b bytes.Buffer
	b.WriteString("SSHSIG")
	b.Write(wireString(SignatureNamespace))
	b.Write(wireString(""))
	b.Write(wireString("sha512"))
	b.Write(wireString(string(h[:])))
	return b.Bytes()
}

func dearmor(armored []byte) ([]byte, error) {
	text := strings.TrimSpace(string(armored))
	const begin, end = "-----BEGIN SSH SIGNATURE-----", "-----END SSH SIGNATURE-----"
	if !strings.HasPrefix(text, begin) || !strings.HasSuffix(text, end) {
		return nil, errSig
	}
	body := strings.Join(strings.Fields(text[len(begin):len(text)-len(end)]), "")
	return base64.StdEncoding.DecodeString(body)
}

func wireString(s string) []byte {
	b := make([]byte, 4+len(s))
	binary.BigEndian.PutUint32(b, uint32(len(s)))
	copy(b[4:], s)
	return b
}

func wireBytes(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// wire reads SSH wire-format fields; any malformed read sets err.
type wire struct {
	b   []byte
	err bool
}

func (w *wire) raw(n int) []byte {
	if w.err || len(w.b) < n {
		w.err = true
		return nil
	}
	out := w.b[:n]
	w.b = w.b[n:]
	return out
}

func (w *wire) u32() uint32 {
	b := w.raw(4)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

func (w *wire) bytes() []byte {
	n := w.u32()
	if n > 1<<20 {
		w.err = true
		return nil
	}
	return w.raw(int(n))
}

func (w *wire) str() string { return string(w.bytes()) }
