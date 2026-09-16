package writer

import "testing"

func TestUseAzureNativeFileCleanup(t *testing.T) {
	chain := &CompactionService{
		dataPath:    "az://homer-data/lake/",
		azureClient: &CompactionAzureClient{AccountName: "acct"},
	}
	if !chain.useAzureNativeFileCleanup() {
		t.Fatal("credential_chain azure writer must use native file cleanup")
	}

	static := &CompactionService{
		dataPath:    "az://homer-data/lake/",
		azureClient: &CompactionAzureClient{AccountName: "acct", AccountKey: "key"},
	}
	if static.useAzureNativeFileCleanup() {
		t.Fatal("account_key azure writer must keep the DuckDB CALL path")
	}

	local := &CompactionService{dataPath: "/var/lib/homer/data"}
	if local.useAzureNativeFileCleanup() {
		t.Fatal("local writer must not use azure native cleanup")
	}
}
