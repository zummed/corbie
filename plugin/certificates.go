package plugin

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
)

// Certificates reads the certificates a script gives to be trusted in
// place of the machine's own, as PEM text. Only those are then trusted.
// One that cannot be read is refused, not passed over: a call that went
// ahead trusting fewer certificates than were written would hide the
// mistake.
func Certificates(text []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	read := 0
	for {
		block, rest := pem.Decode(text)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, Errorf(BadInput, "trust: holds a %s, which is not a certificate", block.Type)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, Errorf(BadInput, "trust: certificate %d cannot be read: %v", read+1, err)
		}
		pool.AddCert(cert)
		read++
		text = rest
	}
	// What is left is text around the certificates, unless it starts one
	// that was not well enough written to be taken apart.
	if bytes.Contains(text, []byte("-----BEGIN")) {
		return nil, Errorf(BadInput, "trust: what follows certificate %d cannot be read as a certificate", read)
	}
	if read == 0 {
		return nil, Errorf(BadInput, "trust: holds no certificate")
	}
	return pool, nil
}
