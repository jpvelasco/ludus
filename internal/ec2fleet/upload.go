package ec2fleet

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jpvelasco/ludus/internal/retry"
)

const (
	// s3MaxSingleUpload is the largest object S3 accepts in a single
	// PutObject request; bigger archives must be uploaded in parts.
	s3MaxSingleUpload = 5 * 1024 * 1024 * 1024 // 5 GB

	// multipartChunkSize is the part size used for large-archive uploads.
	// 64 MB keeps even huge UE packages under S3's 10,000-part limit while
	// amortizing per-part request overhead.
	multipartChunkSize = 64 * 1024 * 1024 // 64 MB
)

// needsMultipartUpload reports whether the archive exceeds the S3
// single-request PutObject limit and must be uploaded in parts.
func needsMultipartUpload(size int64) bool {
	return size > s3MaxSingleUpload
}

// uploadChunk is one byte range of a multipart upload plan.
type uploadChunk struct {
	offset int64
	size   int64
}

// uploadChunkPlan splits size into consecutive chunks of at most chunkSize
// bytes; the final chunk may be smaller. A zero size yields no chunks.
func uploadChunkPlan(size, chunkSize int64) []uploadChunk {
	var chunks []uploadChunk
	for offset := int64(0); offset < size; offset += chunkSize {
		chunks = append(chunks, uploadChunk{offset: offset, size: min(chunkSize, size-offset)})
	}
	return chunks
}

// multipartUpload streams the archive in parts so packages larger than the S3
// single-request limit can deploy. Any failure — including cancellation —
// aborts the incomplete upload so orphaned parts don't accrue storage.
func (d *Deployer) multipartUpload(ctx context.Context, bucket, key string, file io.ReaderAt, chunks []uploadChunk) error {
	createOut, err := d.s3Client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("starting multipart upload: %w", err)
	}
	uploadID := aws.ToString(createOut.UploadId)

	parts := make([]s3types.CompletedPart, 0, len(chunks))
	for i, chunk := range chunks {
		part, err := d.uploadS3Part(ctx, bucket, key, uploadID, i+1, file, chunk)
		if err != nil {
			d.abortMultipartUpload(ctx, bucket, key, uploadID)
			return err
		}
		parts = append(parts, part)
	}

	if _, err := d.s3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{Parts: parts},
	}); err != nil {
		d.abortMultipartUpload(ctx, bucket, key, uploadID)
		return fmt.Errorf("completing multipart upload: %w", err)
	}
	return nil
}

// uploadS3Part sends one chunk (with retries; each attempt gets a fresh
// section reader) and returns its completed-part descriptor.
func (d *Deployer) uploadS3Part(ctx context.Context, bucket, key, uploadID string, partNumber int, file io.ReaderAt, chunk uploadChunk) (s3types.CompletedPart, error) {
	var out *s3.UploadPartOutput
	err := retry.Do(ctx, retry.Default(), func() error {
		var err error
		out, err = d.s3Client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:        aws.String(bucket),
			Key:           aws.String(key),
			UploadId:      aws.String(uploadID),
			PartNumber:    aws.Int32(int32(partNumber)),
			ContentLength: aws.Int64(chunk.size),
			Body:          io.NewSectionReader(file, chunk.offset, chunk.size),
		})
		return err
	})
	if err != nil {
		return s3types.CompletedPart{}, fmt.Errorf("uploading part %d: %w", partNumber, err)
	}
	return s3types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(int32(partNumber))}, nil
}

// abortMultipartUpload best-effort aborts an incomplete multipart upload so
// uploaded parts don't accrue S3 storage. It detaches from the caller's
// cancellation so an interrupted run still cleans up.
func (d *Deployer) abortMultipartUpload(ctx context.Context, bucket, key, uploadID string) {
	if _, err := d.s3Client.AbortMultipartUpload(context.WithoutCancel(ctx), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}); err != nil {
		fmt.Fprintf(d.output(), "  Warning: failed to abort multipart upload: %v\n", err)
		return
	}
	fmt.Fprintln(d.output(), "  Aborted incomplete multipart upload.")
}
