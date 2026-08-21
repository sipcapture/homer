package mover

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
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

type s3Copier struct {
	client *s3.Client
}

func newS3Copier(cfg S3Config) (*s3Copier, error) {
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	awsCfg := aws.Config{Region: region}
	if ak := strings.TrimSpace(cfg.AccessKey); ak != "" {
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider(ak, cfg.SecretKey, "")
	}

	opts := []func(*s3.Options){
		func(o *s3.Options) {
			// Custom endpoints (MinIO/R2/RustFS) need path-style unless the
			// operator set url_style=vhost. Native AWS is virtual-hosted.
			pathStyle := strings.TrimSpace(endpoint) != ""
			switch strings.ToLower(strings.TrimSpace(cfg.URLStyle)) {
			case "vhost":
				pathStyle = false
			case "path":
				pathStyle = true
			}
			o.UsePathStyle = pathStyle
			if endpoint != "" {
				scheme := "https"
				if !cfg.UseSSL {
					scheme = "http"
				}
				o.BaseEndpoint = aws.String(scheme + "://" + endpoint)
			}
		},
	}
	return &s3Copier{client: s3.NewFromConfig(awsCfg, opts...)}, nil
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
	_, err = c.client.PutObject(ctx, in)
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", dstPath, err)
	}
	return nil
}
