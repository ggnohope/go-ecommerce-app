package storage

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type S3Client struct {
	uploader *s3manager.Uploader
	bucket   string
	region   string
}

func NewS3Client(region, bucket string) (*S3Client, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: failed to create session: %w", err)
	}
	return &S3Client{
		uploader: s3manager.NewUploader(sess),
		bucket:   bucket,
		region:   region,
	}, nil
}

// UploadFile uploads a file to S3 and returns its public URL.
// The bucket must have public-read ACL or a permissive bucket policy.
func (s *S3Client) UploadFile(key string, data io.Reader, contentType string) (string, error) {
	result, err := s.uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        data,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("s3: upload failed: %w", err)
	}
	return result.Location, nil
}

func (s *S3Client) ProductImageKey(productID uint, filename string) string {
	return fmt.Sprintf("products/%d/%s", productID, filepath.Base(filename))
}
