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
// Precedence mirrors ducklake.BuildAzureSecretSQL: ConnectionString wins if
// set, else AccountKey (paired with AccountName), else the Azure SDK's
// default credential chain (resolves Managed Identity when running on an
// Azure VM). Endpoint overrides the default public-cloud Blob endpoint
// (Azurite, Gov/China cloud, ...) for the AccountKey and credential_chain
// cases; a raw ConnectionString already carries its own endpoint if needed.
type AzureConfig struct {
	AccountName      string
	AccountKey       string
	ConnectionString string
	Endpoint         string
}

// buildAzureConnectionString is a self-contained copy of
// ducklake.BuildAzureConnectionString: this package cannot import ducklake
// (ducklake already imports mover, so the reverse would be an import
// cycle), so the small connection-string synthesis is duplicated here —
// same convention already used for isS3Path/joinLake between the two
// packages. Keep in sync with ducklake.BuildAzureConnectionString.
func buildAzureConnectionString(accountName, accountKey, endpoint string) string {
	accountName = strings.TrimSpace(accountName)
	accountKey = strings.TrimSpace(accountKey)
	endpoint = strings.TrimSpace(endpoint)
	if endpoint != "" {
		return fmt.Sprintf("AccountName=%s;AccountKey=%s;BlobEndpoint=%s;", accountName, accountKey, endpoint)
	}
	return fmt.Sprintf(
		"DefaultEndpointsProtocol=https;AccountName=%s;AccountKey=%s;EndpointSuffix=core.windows.net",
		accountName, accountKey,
	)
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
	endpoint := strings.TrimSpace(cfg.Endpoint)

	switch {
	case connStr != "":
		return azblob.NewClientFromConnectionString(connStr, nil)
	case accountKey != "":
		if accountName == "" {
			return nil, fmt.Errorf("azure: account_name is required with account_key")
		}
		// Routed through a synthesized connection string (not
		// NewClientWithSharedKeyCredential + a hardcoded public URL) so this
		// always agrees with what buildAzureSecretSQL gives DuckDB, endpoint
		// override included — this is what makes account_key work against
		// Azurite/Gov/China cloud in the native mover, not just the default
		// (duckdb-engine) move path.
		return azblob.NewClientFromConnectionString(buildAzureConnectionString(accountName, accountKey, endpoint), nil)
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
		serviceURL := endpoint
		if serviceURL == "" {
			serviceURL = azureServiceURL(accountName)
		}
		return azblob.NewClient(serviceURL, cred, nil)
	}
}

func azureServiceURL(accountName string) string {
	return fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
}

// azureBlockSize and azureConcurrency mirror s3MultipartPartSize /
// s3Copier's manager.Uploader concurrency (mover/s3.go) so the two native
// copiers behave the same way under load.
const (
	azureBlockSize   = 8 << 20 // 8 MiB
	azureConcurrency = 3
)

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

	// s3Copier passes size as PutObjectInput.ContentLength, which S3
	// validates during the request itself. UploadFileOptions has no
	// equivalent field — it reads size directly off the *os.File handle —
	// so there is no way to get that same request-time validation here.
	// Check the source file's on-disk size against the catalog's recorded
	// size up front instead, mirroring what LocalCopier's n != size check
	// protects against (a truncated/resized source), just checked before
	// the upload rather than during it.
	if size > 0 {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if info.Size() != size {
			return fmt.Errorf("source is %d bytes, catalog size %d: %s", info.Size(), size, srcPath)
		}
	}

	opts := &azblob.UploadFileOptions{
		BlockSize:   azureBlockSize,
		Concurrency: azureConcurrency,
	}
	if _, err := c.client.UploadFile(ctx, container, key, f, opts); err != nil {
		return fmt.Errorf("azure upload %s: %w", dstPath, err)
	}
	return nil
}
