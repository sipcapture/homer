package mover

import (
	"context"
	"testing"
)

func TestS3PathStyle(t *testing.T) {
	cases := []struct {
		endpoint, style string
		want            bool
	}{
		{endpoint: "rustfs:9000", style: "", want: true},
		{endpoint: "127.0.0.1:9000", style: "path", want: true},
		{endpoint: "s3.amazonaws.com", style: "vhost", want: false},
		{endpoint: "", style: "", want: false},
		{endpoint: "", style: "path", want: true},
	}
	for _, tc := range cases {
		if got := s3PathStyle(tc.endpoint, tc.style); got != tc.want {
			t.Errorf("s3PathStyle(%q, %q)=%v want %v", tc.endpoint, tc.style, got, tc.want)
		}
	}
}

func TestS3BaseEndpoint(t *testing.T) {
	if got := s3BaseEndpoint("http://rustfs:9000", false); got != "http://rustfs:9000" {
		t.Fatalf("got %q", got)
	}
	if got := s3BaseEndpoint("minio.local:9000", true); got != "https://minio.local:9000" {
		t.Fatalf("got %q", got)
	}
	if got := s3BaseEndpoint("", true); got != "" {
		t.Fatalf("empty endpoint: %q", got)
	}
}

func TestNewS3CopierStaticCredentials(t *testing.T) {
	c, err := newS3Copier(S3Config{
		Region:    "us-east-1",
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Endpoint:  "127.0.0.1:9000",
		UseSSL:    false,
	})
	if err != nil {
		t.Fatalf("newS3Copier: %v", err)
	}
	creds, err := c.client.Options().Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if creds.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Fatalf("access key = %q", creds.AccessKeyID)
	}
	if !c.client.Options().UsePathStyle {
		t.Fatal("custom endpoint should use path-style")
	}
}
