package mover

import "testing"

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
