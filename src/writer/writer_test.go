package writer

import (
	"testing"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

// TestApplyAzureDuckLakeConfig_AccountNameOnly is a regression test for PR
// review should-fix (github.com/sipcapture/homer/pull/983): "no writer/CLI
// wiring test that Azure is copied when only account_name is set (MI)".
// The Managed Identity case has no account_key and no connection_string —
// only account_name — so a gate that checked the wrong fields (or was
// narrowed by a future edit) could silently drop it, leaving the writer
// with no Azure secret at all despite the operator having configured MI.
func TestApplyAzureDuckLakeConfig_AccountNameOnly(t *testing.T) {
	var duckCfg ducklake.Config
	applyAzureDuckLakeConfig(&duckCfg, config.AzureConfig{AccountName: "myaccount"})

	if duckCfg.AzureAccountName != "myaccount" {
		t.Errorf("AzureAccountName = %q, want %q", duckCfg.AzureAccountName, "myaccount")
	}
	if duckCfg.AzureAccountKey != "" || duckCfg.AzureConnectionString != "" {
		t.Errorf("expected no key/connection string, got key=%q connstr=%q",
			duckCfg.AzureAccountKey, duckCfg.AzureConnectionString)
	}
}

func TestApplyAzureDuckLakeConfig_AllFields(t *testing.T) {
	var duckCfg ducklake.Config
	applyAzureDuckLakeConfig(&duckCfg, config.AzureConfig{
		AccountName:      "myaccount",
		AccountKey:       "mykey",
		ConnectionString: "myconnstr",
		Endpoint:         "https://myaccount.blob.core.windows.net",
	})

	if duckCfg.AzureAccountName != "myaccount" ||
		duckCfg.AzureAccountKey != "mykey" ||
		duckCfg.AzureConnectionString != "myconnstr" ||
		duckCfg.AzureEndpoint != "https://myaccount.blob.core.windows.net" {
		t.Errorf("fields not copied through: %+v", duckCfg)
	}
}

func TestApplyAzureDuckLakeConfig_AllEmptyIsNoOp(t *testing.T) {
	var duckCfg ducklake.Config
	applyAzureDuckLakeConfig(&duckCfg, config.AzureConfig{})

	if duckCfg.AzureAccountName != "" || duckCfg.AzureAccountKey != "" ||
		duckCfg.AzureConnectionString != "" || duckCfg.AzureEndpoint != "" {
		t.Errorf("expected no-op for empty AzureConfig, got %+v", duckCfg)
	}
}
