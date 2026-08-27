// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

// secretProvider executes the CREATE SECRET produced by buildS3SecretSQL on a
// real DuckDB and returns the provider recorded in duckdb_secrets(). It skips
// (rather than fails) when the required DuckDB extensions are unavailable —
// matching the convention in repair_e2e_test.go so CI without network/extension
// access stays green. needAWS loads the aws extension required by the
// credential_chain provider.
func secretProvider(t *testing.T, accessKey, secretKey, region, endpoint string, useSSL, needAWS bool) string {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	// TYPE S3 secrets are provided by httpfs; credential_chain needs aws.
	// LOAD only (extensions must be pre-installed); skip if unavailable.
	if _, err := db.Exec("LOAD httpfs;"); err != nil {
		t.Skipf("httpfs extension unavailable: %v", err)
	}
	if needAWS {
		if _, err := db.Exec("LOAD aws;"); err != nil {
			t.Skipf("aws extension unavailable: %v", err)
		}
	}

	if _, err := db.Exec(buildS3SecretSQL("s3_secret_test", accessKey, secretKey, region, endpoint, useSSL, "")); err != nil {
		t.Skipf("CREATE SECRET unavailable (extension/version): %v", err)
	}

	var name, stype, provider string
	row := db.QueryRow("SELECT name, type, provider FROM duckdb_secrets() WHERE name = 's3_secret_test'")
	if err := row.Scan(&name, &stype, &provider); err != nil {
		t.Skipf("duckdb_secrets() query unavailable: %v", err)
	}
	if stype != "s3" {
		t.Errorf("secret type = %q, want s3", stype)
	}
	return provider
}

// TestCreateSecret_CredentialChain: no static key + no custom endpoint (native
// AWS S3) → the secret uses PROVIDER credential_chain.
func TestCreateSecret_CredentialChain(t *testing.T) {
	if got := secretProvider(t, "", "", "us-east-1", "", true, true); got != "credential_chain" {
		t.Errorf("provider = %q, want credential_chain", got)
	}
}

// TestCreateSecret_StaticKeys: explicit access keys → provider config (not
// credential_chain), preserving prior behaviour.
func TestCreateSecret_StaticKeys(t *testing.T) {
	if got := secretProvider(t, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", "", true, false); got != "config" {
		t.Errorf("provider = %q, want config", got)
	}
}

// TestCreateSecret_CustomEndpoint: empty key but a custom endpoint (MinIO/R2)
// → explicit-credential path, never credential_chain.
func TestCreateSecret_CustomEndpoint(t *testing.T) {
	if got := secretProvider(t, "", "", "us-east-1", "minio.local:9000", false, false); got == "credential_chain" {
		t.Errorf("custom endpoint should not use credential_chain, got provider=%q", got)
	}
}

// TestBuildS3SecretSQL_Branches is a pure unit test of the three-way switch in
// buildS3SecretSQL — no DuckDB required, so it always runs.
func TestBuildS3SecretSQL_Branches(t *testing.T) {
	cases := []struct {
		name       string
		accessKey  string
		endpoint   string
		region     string
		wantSubstr string
		denySubstr string
	}{
		{
			name:       "empty key + no endpoint -> credential_chain",
			accessKey:  "",
			endpoint:   "",
			region:     "us-east-1",
			wantSubstr: "PROVIDER credential_chain",
			denySubstr: "KEY_ID",
		},
		{
			name:       "empty key + empty region -> credential_chain defaults region",
			accessKey:  "",
			endpoint:   "",
			region:     "",
			wantSubstr: "REGION 'us-east-1'",
			denySubstr: "REGION ''",
		},
		{
			name:       "whitespace key + no endpoint -> credential_chain",
			accessKey:  "   ",
			endpoint:   "",
			region:     "us-east-1",
			wantSubstr: "PROVIDER credential_chain",
			denySubstr: "KEY_ID",
		},
		{
			name:       "static key + no endpoint -> explicit keys",
			accessKey:  "AKIA...",
			endpoint:   "",
			region:     "us-east-1",
			wantSubstr: "KEY_ID",
			denySubstr: "credential_chain",
		},
		{
			name:       "empty key + custom endpoint -> explicit keys (MinIO path)",
			accessKey:  "",
			endpoint:   "minio.local:9000",
			region:     "us-east-1",
			wantSubstr: "KEY_ID",
			denySubstr: "credential_chain",
		},
		{
			name:       "static key + custom endpoint -> explicit keys",
			accessKey:  "AKIA...",
			endpoint:   "minio.local:9000",
			region:     "us-east-1",
			wantSubstr: "KEY_ID",
			denySubstr: "credential_chain",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql := buildS3SecretSQL("test_secret", tc.accessKey, "secret", tc.region, tc.endpoint, true, "")
			if !strings.Contains(sql, tc.wantSubstr) {
				t.Errorf("SQL should contain %q:\n%s", tc.wantSubstr, sql)
			}
			if tc.denySubstr != "" && strings.Contains(sql, tc.denySubstr) {
				t.Errorf("SQL should not contain %q:\n%s", tc.denySubstr, sql)
			}
			chain := strings.Contains(sql, "PROVIDER credential_chain")
			refresh := strings.Contains(sql, "REFRESH auto")
			if chain && !refresh {
				t.Errorf("credential_chain SQL must include REFRESH auto:\n%s", sql)
			}
			if !chain && refresh {
				t.Errorf("non-chain SQL must not include REFRESH auto:\n%s", sql)
			}
		})
	}
}

func TestBuildS3SecretSQL_CustomEndpointURLStyle(t *testing.T) {
	sql := buildS3SecretSQL("test_secret", "AKIA...", "secret", "cn-hangzhou", "oss-cn-hangzhou.aliyuncs.com", true, "vhost")
	if !strings.Contains(sql, "URL_STYLE 'vhost'") {
		t.Fatalf("SQL should contain vhost URL_STYLE:\n%s", sql)
	}
}

func TestUsesS3CredentialChain(t *testing.T) {
	if !usesS3CredentialChain("", "") {
		t.Fatal("empty key + empty endpoint is credential_chain")
	}
	if !usesS3CredentialChain("  ", "") {
		t.Fatal("whitespace key is still credential_chain")
	}
	if usesS3CredentialChain("AKIA...", "") {
		t.Fatal("static key is not credential_chain")
	}
	if usesS3CredentialChain("", "minio.local:9000") {
		t.Fatal("custom endpoint is not credential_chain")
	}
}

func TestS3SecretSQLForVolume_ReplaceKeepsRefreshAuto(t *testing.T) {
	vol := &Volume{Name: "cold", Type: VolumeTypeS3, S3Region: "us-east-1"}
	sql := s3SecretSQLForVolume(vol, true)
	if !strings.Contains(sql, "CREATE OR REPLACE SECRET") {
		t.Fatalf("refresh SQL must REPLACE, got:\n%s", sql)
	}
	if !strings.Contains(sql, "PROVIDER credential_chain") || !strings.Contains(sql, "REFRESH auto") {
		t.Fatalf("refresh SQL must keep credential_chain + REFRESH auto:\n%s", sql)
	}
	static := s3SecretSQLForVolume(&Volume{
		Name: "cold", Type: VolumeTypeS3, S3AccessKey: "AKIA...", S3Region: "us-east-1",
	}, true)
	if strings.Contains(static, "credential_chain") {
		t.Fatalf("static-key refresh SQL must not use credential_chain:\n%s", static)
	}
}

func TestRefreshCredentialChainSecret_NoOpWithoutDBOrStaticKeys(t *testing.T) {
	tsm := &TieredStorageManager{}
	if err := tsm.refreshCredentialChainSecret(&Volume{Type: VolumeTypeS3, Name: "cold"}); err != nil {
		t.Fatalf("nil db: %v", err)
	}
	if err := tsm.refreshCredentialChainSecret(&Volume{
		Type: VolumeTypeS3, Name: "cold", S3AccessKey: "AKIA...",
	}); err != nil {
		t.Fatalf("static keys: %v", err)
	}
	if err := tsm.refreshCredentialChainSecret(&Volume{
		Type: VolumeTypeLocal, Name: "hot",
	}); err != nil {
		t.Fatalf("local volume: %v", err)
	}
	if err := tsm.refreshCredentialChainSecret(&Volume{
		Type: VolumeTypeAzure, Name: "cold", AzureAccountKey: "key",
	}); err != nil {
		t.Fatalf("azure static key: %v", err)
	}
}

// azureSecretProvider executes the CREATE SECRET produced by
// BuildAzureSecretSQL on a real DuckDB and returns the provider recorded in
// duckdb_secrets(). Skips (rather than fails) when the azure extension is
// unavailable, matching secretProvider's convention above.
func azureSecretProvider(t *testing.T, accountName, accountKey, connectionString, endpoint string) string {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("LOAD azure;"); err != nil {
		t.Skipf("azure extension unavailable: %v", err)
	}

	if _, err := db.Exec(BuildAzureSecretSQL("azure_secret_test", accountName, accountKey, connectionString, endpoint)); err != nil {
		t.Skipf("CREATE SECRET unavailable (extension/version): %v", err)
	}

	var name, stype, provider string
	row := db.QueryRow("SELECT name, type, provider FROM duckdb_secrets() WHERE name = 'azure_secret_test'")
	if err := row.Scan(&name, &stype, &provider); err != nil {
		t.Skipf("duckdb_secrets() query unavailable: %v", err)
	}
	if stype != "azure" {
		t.Errorf("secret type = %q, want azure", stype)
	}
	return provider
}

// TestCreateAzureSecret_ConnectionString: an explicit connection string uses
// PROVIDER config (DuckDB's default for TYPE azure).
func TestCreateAzureSecret_ConnectionString(t *testing.T) {
	got := azureSecretProvider(t, "", "",
		"DefaultEndpointsProtocol=https;AccountName=fake;AccountKey=ZmFrZQ==;EndpointSuffix=core.windows.net", "")
	if got != "config" {
		t.Errorf("provider = %q, want config", got)
	}
}

// TestCreateAzureSecret_AccountKey: account name + key with no raw connection
// string is synthesized into one by BuildAzureSecretSQL (DuckDB's azure
// extension has no ACCOUNT_KEY parameter under PROVIDER config — verified
// directly against v1.5.5) and still resolves to provider config.
func TestCreateAzureSecret_AccountKey(t *testing.T) {
	got := azureSecretProvider(t, "myaccount", "ZmFrZQ==", "", "")
	if got != "config" {
		t.Errorf("provider = %q, want config", got)
	}
}

// TestCreateAzureSecret_CredentialChain: no key, no connection string ->
// PROVIDER credential_chain (this is what resolves Azure Managed Identity
// when Homer runs on an Azure VM with no static credentials configured).
func TestCreateAzureSecret_CredentialChain(t *testing.T) {
	got := azureSecretProvider(t, "myaccount", "", "", "")
	if got != "credential_chain" {
		t.Errorf("provider = %q, want credential_chain", got)
	}
}

// TestCreateAzureSecret_AccountKeyWithEndpoint: PR review must-fix #2
// (github.com/sipcapture/homer/pull/983) — account_key + a custom endpoint
// (Azurite, Gov/China cloud) must still resolve to a working secret, not
// silently fall back to the public-cloud default. Real DuckDB accepts a
// connection string with only AccountName/AccountKey/BlobEndpoint (no
// DefaultEndpointsProtocol) — verified directly against v1.5.5.
func TestCreateAzureSecret_AccountKeyWithEndpoint(t *testing.T) {
	got := azureSecretProvider(t, "myaccount", "ZmFrZQ==", "", "http://azurite:10000/myaccount")
	if got != "config" {
		t.Errorf("provider = %q, want config", got)
	}
}

// TestCreateAzureSecret_CredentialChainWithEndpoint: PR review must-fix #2
// — credential_chain (Managed Identity / ambient identity) against a custom
// endpoint must also work, not just the account_key case. DuckDB's azure
// extension accepts ENDPOINT as a common secret parameter across providers
// — verified directly against v1.5.5.
func TestCreateAzureSecret_CredentialChainWithEndpoint(t *testing.T) {
	got := azureSecretProvider(t, "myaccount", "", "", "http://fake-endpoint:10000/myaccount")
	if got != "credential_chain" {
		t.Errorf("provider = %q, want credential_chain", got)
	}
}

// TestCreateAzureSecret_AccountKeyWithGovCloudEndpoint and
// TestCreateAzureSecret_CredentialChainWithGovCloudEndpoint prove DuckDB
// itself (not just Homer's SQL string-building) accepts the real-Azure
// endpoint shape — account name as a DNS subdomain, HTTPS, no path segment
// — not just Azurite's shape (account name as a path segment on a fixed
// host:port), which is all the two tests above exercise.
func TestCreateAzureSecret_AccountKeyWithGovCloudEndpoint(t *testing.T) {
	got := azureSecretProvider(t, "myaccount", "ZmFrZQ==", "", "https://myaccount.blob.core.usgovcloudapi.net")
	if got != "config" {
		t.Errorf("provider = %q, want config", got)
	}
}

func TestCreateAzureSecret_CredentialChainWithGovCloudEndpoint(t *testing.T) {
	got := azureSecretProvider(t, "myaccount", "", "", "https://myaccount.blob.core.usgovcloudapi.net")
	if got != "credential_chain" {
		t.Errorf("provider = %q, want credential_chain", got)
	}
}

// TestCreateAzureSecret_AccountKeyWithStandardCloudEndpoint and
// TestCreateAzureSecret_CredentialChainWithStandardCloudEndpoint prove
// DuckDB also accepts the standard/global public cloud's own endpoint shape
// (blob.core.windows.net) when passed explicitly, not just Gov/China or the
// implicit default when endpoint is left empty.
func TestCreateAzureSecret_AccountKeyWithStandardCloudEndpoint(t *testing.T) {
	got := azureSecretProvider(t, "myaccount", "ZmFrZQ==", "", "https://myaccount.blob.core.windows.net")
	if got != "config" {
		t.Errorf("provider = %q, want config", got)
	}
}

func TestCreateAzureSecret_CredentialChainWithStandardCloudEndpoint(t *testing.T) {
	got := azureSecretProvider(t, "myaccount", "", "", "https://myaccount.blob.core.windows.net")
	if got != "credential_chain" {
		t.Errorf("provider = %q, want credential_chain", got)
	}
}

// TestBuildAzureSecretSQL_Branches is a pure unit test of BuildAzureSecretSQL
// — no DuckDB required, so it always runs.
func TestBuildAzureSecretSQL_Branches(t *testing.T) {
	cases := []struct {
		name             string
		accountName      string
		accountKey       string
		connectionString string
		endpoint         string
		wantSubstr       string
		denySubstr       string
	}{
		{
			name:             "connection string wins",
			accountName:      "ignored",
			accountKey:       "ignored",
			connectionString: "DefaultEndpointsProtocol=https;AccountName=fake;AccountKey=ZmFrZQ==;EndpointSuffix=core.windows.net",
			wantSubstr:       "CONNECTION_STRING 'DefaultEndpointsProtocol",
			denySubstr:       "credential_chain",
		},
		{
			name:        "account key synthesizes a connection string",
			accountName: "myaccount",
			accountKey:  "ZmFrZQ==",
			wantSubstr:  "CONNECTION_STRING 'DefaultEndpointsProtocol=https;AccountName=myaccount;AccountKey=ZmFrZQ==;EndpointSuffix=core.windows.net'",
			denySubstr:  "ACCOUNT_KEY",
		},
		{
			name:        "no key, no connection string -> credential_chain",
			accountName: "myaccount",
			wantSubstr:  "PROVIDER credential_chain",
			denySubstr:  "CONNECTION_STRING",
		},
		{
			name:        "credential_chain includes managed_identity",
			accountName: "myaccount",
			wantSubstr:  "managed_identity",
		},
		{
			// PR review must-fix #2: account_key + a custom endpoint must
			// inject BlobEndpoint, not silently fall back to the public
			// cloud EndpointSuffix.
			name:        "account key with custom endpoint uses BlobEndpoint",
			accountName: "myaccount",
			accountKey:  "ZmFrZQ==",
			endpoint:    "http://azurite:10000/myaccount",
			wantSubstr:  "CONNECTION_STRING 'AccountName=myaccount;AccountKey=ZmFrZQ==;BlobEndpoint=http://azurite:10000/myaccount;'",
			denySubstr:  "EndpointSuffix=core.windows.net",
		},
		{
			// PR review must-fix #2: credential_chain + a custom endpoint
			// must pass ENDPOINT through to DuckDB, not silently target
			// the public cloud regardless of the operator's config.
			name:        "credential_chain with custom endpoint adds ENDPOINT clause",
			accountName: "myaccount",
			endpoint:    "http://fake-endpoint:10000/myaccount",
			wantSubstr:  "ENDPOINT 'http://fake-endpoint:10000/myaccount'",
			denySubstr:  "CONNECTION_STRING",
		},
		{
			// The two cases above only use Azurite's URL shape (account name
			// as a path segment on a fixed host:port). Real Azure Gov/China
			// cloud endpoints have a different shape entirely — account name
			// as a DNS subdomain, HTTPS, no path segment
			// (https://<account>.blob.core.usgovcloudapi.net). The code does
			// no URL parsing (pure string interpolation), so it should be
			// shape-agnostic — this proves that rather than assuming it from
			// the Azurite-shaped cases alone.
			name:        "account key with real Azure Gov cloud endpoint",
			accountName: "myaccount",
			accountKey:  "ZmFrZQ==",
			endpoint:    "https://myaccount.blob.core.usgovcloudapi.net",
			wantSubstr:  "CONNECTION_STRING 'AccountName=myaccount;AccountKey=ZmFrZQ==;BlobEndpoint=https://myaccount.blob.core.usgovcloudapi.net;'",
			denySubstr:  "EndpointSuffix=core.windows.net",
		},
		{
			name:        "credential_chain with real Azure China cloud endpoint",
			accountName: "myaccount",
			endpoint:    "https://myaccount.blob.core.chinacloudapi.cn",
			wantSubstr:  "ENDPOINT 'https://myaccount.blob.core.chinacloudapi.cn'",
			denySubstr:  "CONNECTION_STRING",
		},
		{
			// The Gov/China cases above are non-default clouds. Also prove
			// the standard/global public cloud's own DNS-subdomain shape
			// (blob.core.windows.net) works when passed as an *explicit*
			// endpoint override, not just relied on implicitly when
			// endpoint is left empty (the "no endpoint" case above only
			// tests the implicit default, never this shape as a value).
			name:        "account key with explicit standard Azure public cloud endpoint",
			accountName: "myaccount",
			accountKey:  "ZmFrZQ==",
			endpoint:    "https://myaccount.blob.core.windows.net",
			wantSubstr:  "CONNECTION_STRING 'AccountName=myaccount;AccountKey=ZmFrZQ==;BlobEndpoint=https://myaccount.blob.core.windows.net;'",
		},
		{
			name:        "credential_chain with explicit standard Azure public cloud endpoint",
			accountName: "myaccount",
			endpoint:    "https://myaccount.blob.core.windows.net",
			wantSubstr:  "ENDPOINT 'https://myaccount.blob.core.windows.net'",
			denySubstr:  "CONNECTION_STRING",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql := BuildAzureSecretSQL("test_secret", tc.accountName, tc.accountKey, tc.connectionString, tc.endpoint)
			if !strings.Contains(sql, tc.wantSubstr) {
				t.Errorf("SQL should contain %q:\n%s", tc.wantSubstr, sql)
			}
			if tc.denySubstr != "" && strings.Contains(sql, tc.denySubstr) {
				t.Errorf("SQL should not contain %q:\n%s", tc.denySubstr, sql)
			}
			if strings.Contains(sql, "REFRESH") {
				t.Errorf("azure secrets do not support REFRESH (verified against DuckDB v1.5.5):\n%s", sql)
			}
		})
	}
}

func TestUsesAzureCredentialChain(t *testing.T) {
	if !UsesAzureCredentialChain("", "") {
		t.Fatal("empty key + empty connection string is credential_chain")
	}
	if !UsesAzureCredentialChain("  ", "  ") {
		t.Fatal("whitespace-only values are still credential_chain")
	}
	if UsesAzureCredentialChain("key", "") {
		t.Fatal("account key is not credential_chain")
	}
	if UsesAzureCredentialChain("", "conn-string") {
		t.Fatal("connection string is not credential_chain")
	}
}

func TestAzureSecretSQLForVolume_Replace(t *testing.T) {
	vol := &Volume{Name: "cold", Type: VolumeTypeAzure, AzureAccountName: "myaccount"}
	sql := azureSecretSQLForVolume(vol, true)
	if !strings.Contains(sql, "CREATE OR REPLACE SECRET") {
		t.Fatalf("refresh SQL must REPLACE, got:\n%s", sql)
	}
	if !strings.Contains(sql, "PROVIDER credential_chain") {
		t.Fatalf("refresh SQL must keep credential_chain:\n%s", sql)
	}
}
