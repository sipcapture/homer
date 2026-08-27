package mover

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// AzureConfig is the destination volume's Azure Blob Storage settings.
// Precedence mirrors buildAzureSecretSQL: ConnectionString wins if set, else
// AccountKey (paired with AccountName), else the Azure SDK's default
// credential chain (resolves Managed Identity when running on an Azure VM).
type AzureConfig struct {
	AccountName      string
	AccountKey       string
	ConnectionString string
}

type azureCopier struct {
	client *azblob.Client
}

func newAzureCopier(cfg AzureConfig) (*azureCopier, error) {
	client, err := azureBlobClient(cfg)
	if err != nil {
		return nil, err
	}
	return &azureCopier{client: client}, nil
}

func azureBlobClient(cfg AzureConfig) (*azblob.Client, error) {
	connStr := strings.TrimSpace(cfg.ConnectionString)
	accountName := strings.TrimSpace(cfg.AccountName)
	accountKey := strings.TrimSpace(cfg.AccountKey)

	switch {
	case connStr != "":
		return azblob.NewClientFromConnectionString(connStr, nil)
	case accountKey != "":
		if accountName == "" {
			return nil, fmt.Errorf("azure: account_name is required with account_key")
		}
		cred, err := azblob.NewSharedKeyCredential(accountName, accountKey)
		if err != nil {
			return nil, fmt.Errorf("azure: invalid shared key credential: %w", err)
		}
		return azblob.NewClientWithSharedKeyCredential(azureServiceURL(accountName), cred, nil)
	default:
		if accountName == "" {
			return nil, fmt.Errorf("azure: account_name is required for credential_chain auth")
		}
		// DefaultAzureCredential's chain (env, workload identity, managed
		// identity, Azure CLI, ...) is the Go-SDK analog of the DuckDB secret's
		// PROVIDER credential_chain — this is what resolves Managed Identity
		// when Homer runs on an Azure VM with no static keys configured.
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("azure: default credential chain: %w", err)
		}
		return azblob.NewClient(azureServiceURL(accountName), cred, nil)
	}
}

func azureServiceURL(accountName string) string {
	return fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
}

func (c *azureCopier) Copy(ctx context.Context, srcPath, dstPath string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	container, key, ok := splitAzureURL(dstPath)
	if !ok || container == "" || key == "" {
		return fmt.Errorf("invalid azure destination path %q", dstPath)
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := c.client.UploadFile(ctx, container, key, f, nil); err != nil {
		return fmt.Errorf("azure upload %s: %w", dstPath, err)
	}
	return nil
}
