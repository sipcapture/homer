package mover

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config is the destination volume's object-store settings.
type S3Config struct {
	Region    string
	AccessKey string
	SecretKey string
	Endpoint  string
	URLStyle  string
	UseSSL    bool
}

const (
	s3MultipartPartSize = 8 << 20 // 8 MiB; AWS minimum part is 5 MiB
	s3LoadConfigTimeout = 15 * time.Second
)

type s3Copier struct {
	client   *s3.Client
	uploader *manager.Uploader
}

func newS3Copier(cfg S3Config) (*s3Copier, error) {
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	ctx, cancel := context.WithTimeout(context.Background(), s3LoadConfigTimeout)
	defer cancel()

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if ak := strings.TrimSpace(cfg.AccessKey); ak != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(ak, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	pathStyle := s3PathStyle(endpoint, cfg.URLStyle)
	base := s3BaseEndpoint(endpoint, cfg.UseSSL)
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = pathStyle
		if base != "" {
			o.BaseEndpoint = aws.String(base)
			// S3 clones (RustFS/MinIO) often skip AWS flexible checksums.
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
			o.DisableLogOutputChecksumValidationSkipped = true
		}
	})
	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = s3MultipartPartSize
		u.Concurrency = 3
	})
	return &s3Copier{client: client, uploader: uploader}, nil
}

func s3PathStyle(endpoint, urlStyle string) bool {
	pathStyle := strings.TrimSpace(endpoint) != ""
	switch strings.ToLower(strings.TrimSpace(urlStyle)) {
	case "vhost":
		return false
	case "path":
		return true
	}
	return pathStyle
}

func s3BaseEndpoint(endpoint string, useSSL bool) string {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	if endpoint == "" {
		return ""
	}
	scheme := "https"
	if !useSSL {
		scheme = "http"
	}
	return scheme + "://" + endpoint
}

func (c *s3Copier) Copy(ctx context.Context, srcPath, dstPath string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bucket, key, ok := splitS3URL(dstPath)
	if !ok || bucket == "" || key == "" {
		return fmt.Errorf("invalid s3 destination path %q", dstPath)
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	in := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   f,
	}
	if size > 0 {
		in.ContentLength = aws.Int64(size)
	}
	if _, err := c.uploader.Upload(ctx, in); err != nil {
		return fmt.Errorf("s3 upload %s: %w", dstPath, err)
	}
	return nil
}
