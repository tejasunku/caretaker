package webhook

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Certs holds the PEM material and the on-disk key pair paths.
type Certs struct {
	CACert     []byte
	ServerCert []byte
	ServerKey  []byte
	CertFile   string
	KeyFile    string
}

// LoadCerts reads the certificate material that `deploy` placed in the TLS
// Secret. It never generates or writes anything: the in-cluster container
// mounts /certs read-only, and serving is the only thing the pod does with it.
func LoadCerts(dir string) (*Certs, error) {
	caPath := filepath.Join(dir, "ca.crt")
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}
	cert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read server certificate: %w", err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read server key: %w", err)
	}
	if _, err := tls.X509KeyPair(cert, key); err != nil {
		return nil, fmt.Errorf("load server key pair: %w", err)
	}
	return &Certs{CACert: ca, ServerCert: cert, ServerKey: key, CertFile: certPath, KeyFile: keyPath}, nil
}

// EnsureCertsFor loads or creates a CA and server certificate valid for every
// host. Existing certificates are reused only when they already cover all
// requested hosts, so renaming the Service cannot silently produce a stale
// SAN. localhost and 127.0.0.1 are always included.
func EnsureCertsFor(dir string, hosts []string, force bool) (*Certs, error) {
	hosts = cleanHosts(hosts)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("at least one certificate host is required")
	}
	caPath := filepath.Join(dir, "ca.crt")
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")

	if !force {
		ca, caErr := os.ReadFile(caPath)
		cert, certErr := os.ReadFile(certPath)
		key, keyErr := os.ReadFile(keyPath)
		if caErr == nil && certErr == nil && keyErr == nil && certCovers(cert, hosts) {
			return &Certs{CACert: ca, ServerCert: cert, ServerKey: key, CertFile: certPath, KeyFile: keyPath}, nil
		}
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create cert directory: %w", err)
	}
	ca, cert, key, err := generateCerts(hosts)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(caPath, ca, 0o644); err != nil {
		return nil, fmt.Errorf("write CA: %w", err)
	}
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		return nil, fmt.Errorf("write server certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return nil, fmt.Errorf("write server key: %w", err)
	}
	return &Certs{CACert: ca, ServerCert: cert, ServerKey: key, CertFile: certPath, KeyFile: keyPath}, nil
}

func generateCerts(hosts []string) (caCert, serverCert, serverKey []byte, err error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	caSerial, err := serialNumber()
	if err != nil {
		return nil, nil, nil, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "caretaker-webhook-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "caretaker-webhook"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = appendUniqueIP(template.IPAddresses, ip)
		} else {
			template.DNSNames = appendUnique(template.DNSNames, host)
		}
	}
	template.DNSNames = appendUnique(template.DNSNames, "localhost")
	template.IPAddresses = appendUniqueIP(template.IPAddresses, net.ParseIP("127.0.0.1"))
	serverDER, err := x509.CreateCertificate(rand.Reader, template, caTemplate, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	caCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	serverCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKey = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if _, err := tls.X509KeyPair(serverCert, serverKey); err != nil {
		return nil, nil, nil, fmt.Errorf("validate generated key pair: %w", err)
	}
	return caCert, serverCert, serverKey, nil
}

func serialNumber() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func cleanHosts(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host != "" && !contains(out, host) {
			out = append(out, host)
		}
	}
	return out
}

func certCovers(certPEM []byte, hosts []string) bool {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			found := false
			for _, existing := range cert.IPAddresses {
				if existing.Equal(ip) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
			continue
		}
		found := false
		for _, existing := range cert.DNSNames {
			if existing == host {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

func appendUniqueIP(list []net.IP, value net.IP) []net.IP {
	for _, existing := range list {
		if existing.Equal(value) {
			return list
		}
	}
	return append(list, value)
}

func contains(list []string, value string) bool {
	for _, existing := range list {
		if existing == value {
			return true
		}
	}
	return false
}
