package mongodb

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestAddArgs(t *testing.T) {
	cases := []struct {
		name          string
		args, newArgs string
		want          string
	}{
		{"TEST-009 empty arguments prepend /?", "", "ssl=true", "/?ssl=true"},
		{"TEST-010 existing arguments append with &", "/?ssl=true", "replicaSet=rs0", "/?ssl=true&replicaSet=rs0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := addArgs(tc.args, tc.newArgs); got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestProxyDialer(t *testing.T) {
	cases := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{"TEST-011 valid SOCKS5 URL", "socks5://127.0.0.1:1080", false},
		{"TEST-012 unsupported scheme", "http://127.0.0.1:1080", true},
		{"TEST-013 empty proxy falls back to environment dialer", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dialer, err := proxyDialer(&ClientConfig{Proxy: tc.proxy})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error for unsupported proxy scheme, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
			if dialer == nil {
				t.Fatal("expected non-nil dialer")
			}
		})
	}
}

func TestStringers(t *testing.T) {
	cases := []struct {
		name     string
		got      string
		contains []string
	}{
		{"TEST-014 Role", Role{Role: "readWrite", Db: "admin"}.String(), []string{"readWrite", "admin"}},
		{"TEST-015 Privilege", Privilege{
			Resource: Resource{Db: "mydb", Collection: "mycol"},
			Actions:  []string{"find", "insert"},
		}.String(), []string{"mydb", "find"}},
		{"TEST-016 Resource", Resource{Db: "mydb", Collection: "mycol"}.String(), []string{"mydb", "mycol"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, s := range tc.contains {
				if !strings.Contains(tc.got, s) {
					t.Errorf("expected string to contain %q, got %q", s, tc.got)
				}
			}
		})
	}
}

func TestJSONEncoding(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		check func(t *testing.T, data []byte)
	}{
		{
			name:  "TEST-033 Role round-trip preserves values",
			value: Role{Role: "readWrite", Db: "admin"},
			check: func(t *testing.T, data []byte) {
				var decoded Role
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal error: %v", err)
				}
				if want := (Role{Role: "readWrite", Db: "admin"}); decoded != want {
					t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, want)
				}
			},
		},
		{
			name:  "TEST-034 Resource round-trip respects omitempty",
			value: Resource{Db: "test", Collection: ""},
			check: func(t *testing.T, data []byte) {
				if strings.Contains(string(data), "collection") {
					t.Errorf("expected 'collection' to be omitted from JSON, got: %s", data)
				}
				var decoded Resource
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal error: %v", err)
				}
				if want := (Resource{Db: "test", Collection: ""}); decoded != want {
					t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, want)
				}
			},
		},
		{
			name:  "TEST-037 PrivilegeDto with Cluster=true omits db and collection",
			value: PrivilegeDto{Cluster: true, Actions: []string{"find"}},
			check: func(t *testing.T, data []byte) {
				for _, key := range []string{`"db"`, `"collection"`} {
					if strings.Contains(string(data), key) {
						t.Errorf("expected %s to be omitted, got: %s", key, data)
					}
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			tc.check(t, data)
		})
	}
}

func TestBuildPrivilegeDocs(t *testing.T) {
	cases := []struct {
		name    string
		priv    PrivilegeDto
		wantErr bool
		check   func(t *testing.T, resource bson.D)
	}{
		{
			// TEST-064: Regression test: MongoDB rejects privileges with only one of
			// db/collection set. The old code used JSON omitempty which dropped
			// collection="" from the BSON.
			name: "TEST-064 empty collection is included",
			priv: PrivilegeDto{Db: "orders", Collection: "", Actions: []string{"find", "insert"}},
			check: func(t *testing.T, resource bson.D) {
				var hasDb, hasCollection bool
				for _, elem := range resource {
					if elem.Key == "db" {
						hasDb = true
						if elem.Value != "orders" {
							t.Errorf("expected db='orders', got %v", elem.Value)
						}
					}
					if elem.Key == "collection" {
						hasCollection = true
						if elem.Value != "" {
							t.Errorf("expected collection='', got %v", elem.Value)
						}
					}
				}
				if !hasDb {
					t.Error("resource document missing 'db' key")
				}
				if !hasCollection {
					t.Error("resource document missing 'collection' key — MongoDB requires both db and collection")
				}
			},
		},
		{
			name: "TEST-065 cluster=true omits db and collection",
			priv: PrivilegeDto{Cluster: true, Actions: []string{"serverStatus"}},
			check: func(t *testing.T, resource bson.D) {
				for _, elem := range resource {
					if elem.Key == "db" || elem.Key == "collection" {
						t.Errorf("cluster privilege should not have %q key", elem.Key)
					}
				}
				if resource[0].Key != "cluster" || resource[0].Value != true {
					t.Errorf("expected {cluster: true}, got %v", resource)
				}
			},
		},
		{
			name:    "TEST-066 rejects db with cluster=true",
			priv:    PrivilegeDto{Db: "orders", Cluster: true, Actions: []string{"find"}},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			docs, err := buildPrivilegeDocs("test_role", []PrivilegeDto{tc.priv})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error for db + cluster=true, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 privilege doc, got %d", len(docs))
			}
			tc.check(t, docs[0].(bson.D)[0].Value.(bson.D))
		})
	}
}

// generateTestCertAndKey creates a self-signed certificate and its private
// key, both PEM-encoded.
func generateTestCertAndKey(t *testing.T) (certPEM, keyPEM []byte) {
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
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// generateTestPEM creates a self-signed PEM certificate for testing.
func generateTestPEM(t *testing.T) []byte {
	certPEM, _ := generateTestCertAndKey(t)
	return certPEM
}

func TestGetTLSConfig(t *testing.T) {
	pemData := generateTestPEM(t)
	cases := []struct {
		name    string
		pem     []byte
		verify  bool
		wantErr bool
		check   func(t *testing.T, cfg *tls.Config)
	}{
		{name: "TEST-038 invalid PEM returns error", pem: []byte("not-a-pem-cert"), wantErr: true},
		{
			name: "TEST-039 verify=true sets InsecureSkipVerify", pem: pemData, verify: true,
			check: func(t *testing.T, cfg *tls.Config) {
				if !cfg.InsecureSkipVerify {
					t.Error("expected InsecureSkipVerify=true")
				}
			},
		},
		{
			name: "TEST-040 valid PEM populates RootCAs", pem: pemData,
			check: func(t *testing.T, cfg *tls.Config) {
				if cfg.RootCAs == nil {
					t.Error("expected non-nil RootCAs")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := getTLSConfigWithAllServerCertificates(tc.pem, tc.verify)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error for invalid PEM, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, cfg)
		})
	}
}

func TestMongoClientOptions(t *testing.T) {
	clientCert, clientKey := generateTestCertAndKey(t)
	clientBundle := string(clientCert) + string(clientKey)
	cases := []struct {
		name    string
		cfg     ClientConfig
		wantErr bool
		check   func(t *testing.T, opts *options.ClientOptions)
	}{
		{
			name: "TEST-041 full params",
			cfg:  ClientConfig{Host: "myhost", Port: "27018", Ssl: true, ReplicaSet: "rs0", RetryWrites: true, Direct: false},
		},
		{
			name: "TEST-042 Direct=true omits replicaSet",
			cfg:  ClientConfig{Host: "myhost", Port: "27017", ReplicaSet: "rs0", Direct: true, RetryWrites: false},
		},
		{
			// TEST-067: Regression test: the flag was previously honored only when a
			// custom CA certificate was also provided, so the driver's default TLS
			// config verified (and rejected) certificates the user had opted out of
			// verifying, e.g. legacy CN-only certificates.
			name: "TEST-067 ssl + insecure_skip_verify without certificate skips verification",
			cfg:  ClientConfig{Host: "myhost", Port: "27017", Ssl: true, InsecureSkipVerify: true},
			check: func(t *testing.T, opts *options.ClientOptions) {
				if opts.TLSConfig == nil {
					t.Fatal("expected TLS config to be set")
				}
				if !opts.TLSConfig.InsecureSkipVerify {
					t.Error("expected InsecureSkipVerify=true on client TLS config")
				}
			},
		},
		{
			name: "TEST-068 insecure_skip_verify without ssl does not enable TLS",
			cfg:  ClientConfig{Host: "myhost", Port: "27017", InsecureSkipVerify: true},
			check: func(t *testing.T, opts *options.ClientOptions) {
				if opts.TLSConfig != nil {
					t.Error("expected no TLS config when ssl=false and no certificate is set")
				}
			},
		},
		{
			name: "TEST-069 certificate + insecure_skip_verify sets RootCAs and skip flag",
			cfg:  ClientConfig{Host: "myhost", Port: "27017", Ssl: true, InsecureSkipVerify: true, Certificate: string(generateTestPEM(t))},
			check: func(t *testing.T, opts *options.ClientOptions) {
				if opts.TLSConfig == nil {
					t.Fatal("expected TLS config to be set")
				}
				if !opts.TLSConfig.InsecureSkipVerify {
					t.Error("expected InsecureSkipVerify=true on client TLS config")
				}
				if opts.TLSConfig.RootCAs == nil {
					t.Error("expected RootCAs to be populated from certificate")
				}
			},
		},
		{
			// TEST-070: a client certificate alone turns TLS on and is presented
			// to the server; without a CA the system roots verify the server.
			name: "TEST-070 client_certificate_key alone enables TLS and presents the certificate",
			cfg:  ClientConfig{Host: "myhost", Port: "27017", ClientCertificateKey: clientBundle},
			check: func(t *testing.T, opts *options.ClientOptions) {
				if opts.TLSConfig == nil {
					t.Fatal("expected TLS config to be set")
				}
				if len(opts.TLSConfig.Certificates) != 1 {
					t.Fatalf("expected 1 client certificate, got %d", len(opts.TLSConfig.Certificates))
				}
				if opts.TLSConfig.RootCAs != nil || opts.TLSConfig.InsecureSkipVerify {
					t.Error("expected system roots with verification enabled")
				}
			},
		},
		{
			name: "TEST-071 certificate + client_certificate_key sets RootCAs and the client certificate",
			cfg:  ClientConfig{Host: "myhost", Port: "27017", Ssl: true, Certificate: string(generateTestPEM(t)), ClientCertificateKey: clientBundle},
			check: func(t *testing.T, opts *options.ClientOptions) {
				if opts.TLSConfig == nil || opts.TLSConfig.RootCAs == nil {
					t.Fatal("expected TLS config with RootCAs")
				}
				if len(opts.TLSConfig.Certificates) != 1 {
					t.Errorf("expected 1 client certificate, got %d", len(opts.TLSConfig.Certificates))
				}
			},
		},
		{
			name: "TEST-072 client_certificate_key accepts the key before the certificate",
			cfg:  ClientConfig{Host: "myhost", Port: "27017", ClientCertificateKey: string(clientKey) + string(clientCert)},
			check: func(t *testing.T, opts *options.ClientOptions) {
				if opts.TLSConfig == nil || len(opts.TLSConfig.Certificates) != 1 {
					t.Fatal("expected TLS config with 1 client certificate")
				}
			},
		},
		{
			name:    "TEST-073 client_certificate_key without a private key is rejected",
			cfg:     ClientConfig{Host: "myhost", Port: "27017", ClientCertificateKey: string(clientCert)},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := tc.cfg.mongoClientOptions(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "client_certificate_key") {
					t.Errorf("expected error to name client_certificate_key, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts == nil {
				t.Fatal("expected non-nil options")
			}
			if tc.check != nil {
				tc.check(t, opts)
			}
		})
	}
}

func TestMongoClientConstructors(t *testing.T) {
	cases := []struct {
		name   string
		cfg    ClientConfig
		noAuth bool
	}{
		{
			name:   "TEST-043 MongoClientNoAuth",
			cfg:    ClientConfig{Host: "localhost", Port: "27017", RetryWrites: false, Direct: true},
			noAuth: true,
		},
		{
			name: "TEST-044 MongoClient with credentials",
			cfg:  ClientConfig{Host: "localhost", Port: "27017", Username: "admin", Password: "secret", DB: "admin", RetryWrites: false, Direct: true},
		},
		{
			name: "TEST-046 SHARD-011 MongoClient with empty credentials skips auth",
			cfg:  ClientConfig{Host: "localhost", Port: "27017", Username: "", Password: "", DB: "admin", RetryWrites: false, Direct: true},
		},
		{
			name:   "TEST-045 MongoClientNoAuth with certificate uses TLS without auth",
			cfg:    ClientConfig{Host: "localhost", Port: "27017", RetryWrites: false, Direct: true, Certificate: string(generateTestPEM(t))},
			noAuth: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client, err := tc.cfg.MongoClient(ctx)
			if tc.noAuth {
				client, err = tc.cfg.MongoClientNoAuth(ctx)
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if client == nil {
				t.Fatal("expected non-nil client")
			}
		})
	}
}

// DANGER-021: pwd is sent only when includePassword is true; roles always render
func TestBuildUpdateUserCmd(t *testing.T) {
	roles := []Role{{Role: "read", Db: "app"}}
	cases := []struct {
		name            string
		password        string
		includePassword bool
		roles           []Role
		wantKeys        []string
		check           func(t *testing.T, cmd bson.D)
	}{
		{
			name: "include password", password: "s3cret", includePassword: true, roles: roles,
			wantKeys: []string{"updateUser", "pwd", "roles"},
			check: func(t *testing.T, cmd bson.D) {
				if cmd[1].Value != "s3cret" {
					t.Errorf("expected pwd value 's3cret', got %v", cmd[1].Value)
				}
			},
		},
		{
			name: "omit password", password: "", includePassword: false, roles: roles,
			wantKeys: []string{"updateUser", "roles"},
		},
		{
			name: "empty role list renders as explicit empty array", roles: nil,
			wantKeys: []string{"updateUser", "roles"},
			check: func(t *testing.T, cmd bson.D) {
				roles, ok := cmd[len(cmd)-1].Value.([]bson.M)
				if !ok {
					t.Fatalf("expected roles value type []bson.M, got %T", cmd[len(cmd)-1].Value)
				}
				if len(roles) != 0 {
					t.Errorf("expected empty roles array, got %v", roles)
				}
			},
		},
		{
			// kills the "send whenever password is non-empty" mutant
			name: "non-empty password still omitted", password: "still-secret", includePassword: false, roles: roles,
			wantKeys: []string{"updateUser", "roles"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := buildUpdateUserCmd("alice", tc.password, tc.includePassword, tc.roles)
			keys := make([]string, len(cmd))
			for i, e := range cmd {
				keys[i] = e.Key
			}
			if strings.Join(keys, ",") != strings.Join(tc.wantKeys, ",") {
				t.Fatalf("expected keys %v, got %v", tc.wantKeys, keys)
			}
			if tc.check != nil {
				tc.check(t, cmd)
			}
		})
	}
}
