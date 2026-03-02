package mongodb

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// TEST-009: addArgs with empty arguments prepends "/?"
func TestAddArgs_EmptyArguments(t *testing.T) {
	result := addArgs("", "ssl=true")
	expected := "/?ssl=true"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TEST-010: addArgs with existing arguments appends with "&"
func TestAddArgs_ExistingArguments(t *testing.T) {
	result := addArgs("/?ssl=true", "replicaSet=rs0")
	expected := "/?ssl=true&replicaSet=rs0"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TEST-011: proxyDialer with valid SOCKS5 URL returns non-nil dialer
func TestProxyDialer_ValidSOCKS5(t *testing.T) {
	c := &ClientConfig{Proxy: "socks5://127.0.0.1:1080"}
	dialer, err := proxyDialer(c)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if dialer == nil {
		t.Fatal("expected non-nil dialer")
	}
}

// TEST-012: proxyDialer with unsupported scheme returns error
func TestProxyDialer_InvalidScheme(t *testing.T) {
	c := &ClientConfig{Proxy: "http://127.0.0.1:1080"}
	_, err := proxyDialer(c)
	if err == nil {
		t.Fatal("expected error for unsupported proxy scheme, got nil")
	}
}

// TEST-013: proxyDialer with empty proxy falls back to environment dialer
func TestProxyDialer_EmptyFallback(t *testing.T) {
	c := &ClientConfig{Proxy: ""}
	dialer, err := proxyDialer(c)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if dialer == nil {
		t.Fatal("expected non-nil dialer from environment fallback")
	}
}

// TEST-014: Role.String() contains role and db values
func TestRoleString(t *testing.T) {
	r := Role{Role: "readWrite", Db: "admin"}
	s := r.String()
	if !strings.Contains(s, "readWrite") {
		t.Errorf("expected string to contain 'readWrite', got %q", s)
	}
	if !strings.Contains(s, "admin") {
		t.Errorf("expected string to contain 'admin', got %q", s)
	}
}

// TEST-015: Privilege.String() contains resource and actions
func TestPrivilegeString(t *testing.T) {
	p := Privilege{
		Resource: Resource{Db: "mydb", Collection: "mycol"},
		Actions:  []string{"find", "insert"},
	}
	s := p.String()
	if !strings.Contains(s, "mydb") {
		t.Errorf("expected string to contain 'mydb', got %q", s)
	}
	if !strings.Contains(s, "find") {
		t.Errorf("expected string to contain 'find', got %q", s)
	}
}

// TEST-016: Resource.String() contains db and collection
func TestResourceString(t *testing.T) {
	r := Resource{Db: "mydb", Collection: "mycol"}
	s := r.String()
	if !strings.Contains(s, "mydb") {
		t.Errorf("expected string to contain 'mydb', got %q", s)
	}
	if !strings.Contains(s, "mycol") {
		t.Errorf("expected string to contain 'mycol', got %q", s)
	}
}

// TEST-033: Role JSON round-trip preserves values
func TestRoleJSONRoundTrip(t *testing.T) {
	original := Role{Role: "readWrite", Db: "admin"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded Role
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

// TEST-034: Resource JSON round-trip respects omitempty
func TestResourceJSONOmitEmpty(t *testing.T) {
	original := Resource{Db: "test", Collection: ""}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	jsonStr := string(data)
	if strings.Contains(jsonStr, "collection") {
		t.Errorf("expected 'collection' to be omitted from JSON, got: %s", jsonStr)
	}
	var decoded Resource
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

// TEST-037: PrivilegeDto with Cluster=true omits db and collection
func TestPrivilegeDtoClusterOmitsDbCollection(t *testing.T) {
	p := PrivilegeDto{Cluster: true, Actions: []string{"find"}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	jsonStr := string(data)
	if strings.Contains(jsonStr, `"db"`) {
		t.Errorf("expected 'db' to be omitted, got: %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"collection"`) {
		t.Errorf("expected 'collection' to be omitted, got: %s", jsonStr)
	}
}

// generateTestPEM creates a self-signed PEM certificate for testing.
func generateTestPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

// TEST-038: Invalid PEM returns error
func TestGetTLSConfig_InvalidPEM(t *testing.T) {
	_, err := getTLSConfigWithAllServerCertificates([]byte("not-a-pem-cert"), false)
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
}

// TEST-039: Valid PEM with verify=true sets InsecureSkipVerify=true
func TestGetTLSConfig_VerifyFlag(t *testing.T) {
	pemData := generateTestPEM(t)
	tlsConfig, err := getTLSConfigWithAllServerCertificates(pemData, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tlsConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true")
	}
}

// TEST-040: Valid PEM populates RootCAs
func TestGetTLSConfig_RootCAsPopulated(t *testing.T) {
	pemData := generateTestPEM(t)
	tlsConfig, err := getTLSConfigWithAllServerCertificates(pemData, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsConfig.RootCAs == nil {
		t.Error("expected non-nil RootCAs")
	}
}
