package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Config holds the S3 adapter configuration.
type Config struct {
	Bucket   string
	Region   string
	Endpoint string // For S3-compatible services (MinIO, LocalStack)
}

// Adapter implements port.StorageAdapter for AWS S3 and compatible services.
type Adapter struct {
	client *s3.Client
	bucket string
}

// New creates a new S3 storage adapter.
func New(cfg *Config) (port.StorageAdapter, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("s3: bucket is required")
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("s3: loading aws config: %w", err)
	}

	var clientOpts []func(*s3.Options)

	// Custom endpoint for S3-compatible services (MinIO, LocalStack)
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)

	return &Adapter{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Upload stores data with the given key and content type.
func (a *Adapter) Upload(ctx context.Context, req *port.StorageUploadRequest) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(a.bucket),
		Key:         aws.String(req.Key),
		Body:        bytes.NewReader(req.Data),
		ContentType: aws.String(req.ContentType),
	}

	_, err := a.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3: uploading object: %w", err)
	}

	return nil
}

// Download retrieves data by key.
func (a *Adapter) Download(ctx context.Context, req *port.StorageRequest) ([]byte, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(req.Key),
	}

	result, err := a.client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("s3: getting object: %w", err)
	}
	defer func() { _ = result.Body.Close() }()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("s3: reading object body: %w", err)
	}

	return data, nil
}

// GetURL returns a presigned URL for accessing the object.
func (a *Adapter) GetURL(ctx context.Context, req *port.StorageRequest) (string, error) {
	presignClient := s3.NewPresignClient(a.client)

	input := &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(req.Key),
	}

	// Generate presigned URL valid for 1 hour
	result, err := presignClient.PresignGetObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = time.Hour
	})
	if err != nil {
		return "", fmt.Errorf("s3: presigning url: %w", err)
	}

	return result.URL, nil
}

// Delete removes an object by key.
func (a *Adapter) Delete(ctx context.Context, req *port.StorageRequest) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(req.Key),
	}

	_, err := a.client.DeleteObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3: deleting object: %w", err)
	}

	return nil
}

// Exists checks if an object exists at the given key.
func (a *Adapter) Exists(ctx context.Context, req *port.StorageRequest) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(req.Key),
	}

	_, err := a.client.HeadObject(ctx, input)
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, fmt.Errorf("s3: checking object existence: %w", err)
	}

	return true, nil
}
