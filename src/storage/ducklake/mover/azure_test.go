package mover

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSplitAzureURL(t *testing.T) {
	cases := []struct {
		url       string
		container string
		key       string
		ok        bool
	}{
		{url: "az://mycontainer/path/to/file.parquet", container: "mycontainer", key: "path/to/file.parquet", ok: true},
		{url: "azure://mycontainer/path/to/file.parquet", container: "mycontainer", key: "path/to/file.parquet", ok: true},
		{url: "az://mycontainer", container: "mycontainer", key: "", ok: true},
		{url: "az://mycontainer/", container: "mycontainer", key: "", ok: true},
		{url: "s3://bucket/key", container: "", key: "", ok: false},
		{url: "az://", container: "", key: "", ok: false},
	}
	for _, tc := range cases {
		container, key, ok := splitAzureURL(tc.url)
		if container != tc.container || key != tc.key || ok != tc.ok {
			t.Errorf("splitAzureURL(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.url, container, key, ok, tc.container, tc.key, tc.ok)
		}
	}
}

func TestIsAzurePath(t *testing.T) {
	if !isAzurePath("az://container/path") {
		t.Fatal("az:// should be an azure path")
	}
	if !isAzurePath("azure://container/path") {
		t.Fatal("azure:// should be an azure path")
	}
	if isAzurePath("s3://bucket/path") {
		t.Fatal("s3:// should not be an azure path")
	}
	if isAzurePath("/local/path") {
		t.Fatal("local path should not be an azure path")
	}
}

func TestAzureBlobClient_ConnectionString(t *testing.T) {
	client, err := azureBlobClient(AzureConfig{
		ConnectionString: "DefaultEndpointsProtocol=https;AccountName=fake;AccountKey=ZmFrZQ==;EndpointSuffix=core.windows.net",
	})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestAzureBlobClient_AccountKey(t *testing.T) {
	client, err := azureBlobClient(AzureConfig{
		AccountName: "fakeaccount",
		AccountKey:  "ZmFrZQ==",
	})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestAzureBlobClient_AccountKeyRequiresAccountName(t *testing.T) {
	if _, err := azureBlobClient(AzureConfig{AccountKey: "ZmFrZQ=="}); err == nil {
		t.Fatal("expected error when account_key is set without account_name")
	}
}

func TestAzureBlobClient_CredentialChain(t *testing.T) {
	// No key, no connection string: default credential chain (resolves
	// Managed Identity on an Azure VM). Constructing the credential/client
	// does not itself require network access — only a later GetToken call
	// would — so this must succeed even outside Azure.
	client, err := azureBlobClient(AzureConfig{AccountName: "fakeaccount"})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestAzureBlobClient_CredentialChainRequiresAccountName(t *testing.T) {
	if _, err := azureBlobClient(AzureConfig{}); err == nil {
		t.Fatal("expected error when no key, no connection string, and no account_name")
	}
}

// TestBuildAzureConnectionString: PR review must-fix #2
// (github.com/sipcapture/homer/pull/983) — a custom endpoint must produce a
// BlobEndpoint connection string (Azurite/Gov/China cloud), not silently
// default to the public-cloud EndpointSuffix.
func TestBuildAzureConnectionString(t *testing.T) {
	got := buildAzureConnectionString("myaccount", "ZmFrZQ==", "")
	want := "DefaultEndpointsProtocol=https;AccountName=myaccount;AccountKey=ZmFrZQ==;EndpointSuffix=core.windows.net"
	if got != want {
		t.Errorf("no endpoint: got %q, want %q", got, want)
	}

	got = buildAzureConnectionString("myaccount", "ZmFrZQ==", "http://azurite:10000/myaccount")
	want = "AccountName=myaccount;AccountKey=ZmFrZQ==;BlobEndpoint=http://azurite:10000/myaccount;"
	if got != want {
		t.Errorf("custom endpoint: got %q, want %q", got, want)
	}

	// Real Azure Gov/China cloud endpoints have a different URL shape than
	// Azurite (DNS subdomain, HTTPS, no path segment vs. path-segment
	// account name on a fixed host:port) — the function does no URL
	// parsing, so it should handle both identically. Prove it rather than
	// only exercising the Azurite shape above.
	got = buildAzureConnectionString("myaccount", "ZmFrZQ==", "https://myaccount.blob.core.usgovcloudapi.net")
	want = "AccountName=myaccount;AccountKey=ZmFrZQ==;BlobEndpoint=https://myaccount.blob.core.usgovcloudapi.net;"
	if got != want {
		t.Errorf("gov cloud endpoint: got %q, want %q", got, want)
	}
}

// TestAzureBlobClient_AccountKeyWithEndpoint: the native mover's
// account_key branch must honor a custom endpoint (previously it always
// pointed at https://<account>.blob.core.windows.net/ regardless of any
// endpoint override — PR review must-fix #2).
func TestAzureBlobClient_AccountKeyWithEndpoint(t *testing.T) {
	client, err := azureBlobClient(AzureConfig{
		AccountName: "fakeaccount",
		AccountKey:  "ZmFrZQ==",
		Endpoint:    "http://azurite:10000/fakeaccount",
	})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// NewClientFromConnectionString normalizes with a trailing slash.
	if got := client.URL(); got != "http://azurite:10000/fakeaccount/" {
		t.Errorf("client.URL() = %q, want the configured endpoint", got)
	}
}

// TestAzureBlobClient_CredentialChainWithEndpoint: the credential_chain
// branch must also honor a custom endpoint, not just account_key (PR review
// must-fix #2).
func TestAzureBlobClient_CredentialChainWithEndpoint(t *testing.T) {
	client, err := azureBlobClient(AzureConfig{
		AccountName: "fakeaccount",
		Endpoint:    "http://fake-endpoint:10000/fakeaccount",
	})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if got := client.URL(); got != "http://fake-endpoint:10000/fakeaccount" {
		t.Errorf("client.URL() = %q, want the configured endpoint", got)
	}
}

// TestAzureBlobClient_CredentialChainWithGovCloudEndpoint: the two tests
// above only use Azurite's URL shape. Prove the real-Azure shape (DNS
// subdomain, HTTPS, no path segment) also works, for both mover branches —
// see the equivalent Gov cloud case in tiered_storage_test.go for the
// DuckDB-secret side of the same thing.
func TestAzureBlobClient_CredentialChainWithGovCloudEndpoint(t *testing.T) {
	client, err := azureBlobClient(AzureConfig{
		AccountName: "myaccount",
		Endpoint:    "https://myaccount.blob.core.usgovcloudapi.net",
	})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	if got := client.URL(); got != "https://myaccount.blob.core.usgovcloudapi.net" {
		t.Errorf("client.URL() = %q, want the configured endpoint", got)
	}
}

func TestAzureBlobClient_AccountKeyWithGovCloudEndpoint(t *testing.T) {
	client, err := azureBlobClient(AzureConfig{
		AccountName: "myaccount",
		AccountKey:  "ZmFrZQ==",
		Endpoint:    "https://myaccount.blob.core.usgovcloudapi.net",
	})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	// NewClientFromConnectionString normalizes with a trailing slash.
	if got := client.URL(); got != "https://myaccount.blob.core.usgovcloudapi.net/" {
		t.Errorf("client.URL() = %q, want the configured endpoint", got)
	}
}

// TestAzureBlobClient_CredentialChainWithStandardCloudEndpoint and
// TestAzureBlobClient_AccountKeyWithStandardCloudEndpoint: the Gov cloud
// tests above are a non-default cloud. Also prove the standard/global
// public cloud's own endpoint shape (blob.core.windows.net) works when
// passed explicitly as Endpoint, not just relied on implicitly via the
// no-endpoint default (see TestAzureBlobClient_AccountKey/CredentialChain).
func TestAzureBlobClient_CredentialChainWithStandardCloudEndpoint(t *testing.T) {
	client, err := azureBlobClient(AzureConfig{
		AccountName: "myaccount",
		Endpoint:    "https://myaccount.blob.core.windows.net",
	})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	if got := client.URL(); got != "https://myaccount.blob.core.windows.net" {
		t.Errorf("client.URL() = %q, want the configured endpoint", got)
	}
}

func TestAzureBlobClient_AccountKeyWithStandardCloudEndpoint(t *testing.T) {
	client, err := azureBlobClient(AzureConfig{
		AccountName: "myaccount",
		AccountKey:  "ZmFrZQ==",
		Endpoint:    "https://myaccount.blob.core.windows.net",
	})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	if got := client.URL(); got != "https://myaccount.blob.core.windows.net/" {
		t.Errorf("client.URL() = %q, want the configured endpoint", got)
	}
}

// TestAzureCopier_CopyRejectsSizeMismatch is a regression test for PR
// review nit (github.com/sipcapture/homer/pull/983): azureCopier.Copy
// ignored the size parameter entirely, unlike LocalCopier which verifies
// the copied byte count matches the catalog's recorded size. The check
// here runs before any network call (via os.File.Stat, not after
// uploading), so this test never touches the network and cannot hang —
// a mismatched size must fail fast with a clear error instead of silently
// uploading the wrong-sized file.
func TestAzureCopier_CopyRejectsSizeMismatch(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "azure-copy-test-*.parquet")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString("hello world"); err != nil { // 11 bytes
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	client, err := azureBlobClient(AzureConfig{AccountName: "fakeaccount"})
	if err != nil {
		t.Fatalf("azureBlobClient: %v", err)
	}
	c := &azureCopier{client: client}

	err = c.Copy(context.Background(), f.Name(), "az://mycontainer/dest.parquet", 999)
	if err == nil {
		t.Fatal("expected an error for mismatched size, got nil")
	}
	if !strings.Contains(err.Error(), "11 bytes") || !strings.Contains(err.Error(), "catalog size 999") {
		t.Errorf("error should mention both sizes, got: %v", err)
	}
}
