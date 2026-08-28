package mover

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Copier copies one parquet object from a local source path to a destination
// lake path (local filesystem or s3://). DuckDB is not involved.
type Copier interface {
	Copy(ctx context.Context, srcPath, dstPath string, size int64) error
}

// LocalCopier writes destination files on the local filesystem. Source must be local.
type LocalCopier struct{}

func (LocalCopier) Copy(ctx context.Context, srcPath, dstPath string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if isS3Path(dstPath) || isAzurePath(dstPath) {
		return fmt.Errorf("local copier cannot write %s", dstPath)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if size > 0 && n != size {
		return fmt.Errorf("copied %d bytes, catalog size %d: %s", n, size, srcPath)
	}
	return nil
}

// CopierFunc adapts a function to Copier (tests).
type CopierFunc func(ctx context.Context, srcPath, dstPath string, size int64) error

func (f CopierFunc) Copy(ctx context.Context, srcPath, dstPath string, size int64) error {
	return f(ctx, srcPath, dstPath, size)
}

func defaultCopier(dstDataPath string, s3cfg *S3Config, azurecfg *AzureConfig) (Copier, error) {
	if isS3Path(dstDataPath) {
		cfg := S3Config{}
		if s3cfg != nil {
			cfg = *s3cfg
		}
		return newS3Copier(cfg)
	}
	if isAzurePath(dstDataPath) {
		cfg := AzureConfig{}
		if azurecfg != nil {
			cfg = *azurecfg
		}
		return newAzureCopier(cfg)
	}
	return LocalCopier{}, nil
}
