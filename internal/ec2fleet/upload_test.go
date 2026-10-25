package ec2fleet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// fakeS3Client implements s3API with canned responses and call recording so
// the upload decision and the multipart lifecycle can be driven hermetically.
type fakeS3Client struct {
	putObjectErr        error
	createMultipartOut  *s3.CreateMultipartUploadOutput
	createMultipartErr  error
	uploadPartOut       *s3.UploadPartOutput
	uploadPartErr       error
	completeMultipartEr error
	abortMultipartErr   error

	putCalls             int
	createMultipartCalls int
	uploadPartInputs     []*s3.UploadPartInput
	uploadPartBodies     [][]byte
	completeInputs       []*s3.CompleteMultipartUploadInput
	abortInputs          []*s3.AbortMultipartUploadInput
}

func (f *fakeS3Client) PutObject(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putCalls++
	return &s3.PutObjectOutput{}, f.putObjectErr
}

func (f *fakeS3Client) CreateMultipartUpload(_ context.Context, in *s3.CreateMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	f.createMultipartCalls++
	if f.createMultipartOut != nil {
		return f.createMultipartOut, f.createMultipartErr
	}
	return &s3.CreateMultipartUploadOutput{UploadId: aws.String("upload-1")}, f.createMultipartErr
}

func (f *fakeS3Client) UploadPart(_ context.Context, in *s3.UploadPartInput, _ ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	body, _ := io.ReadAll(in.Body)
	f.uploadPartInputs = append(f.uploadPartInputs, in)
	f.uploadPartBodies = append(f.uploadPartBodies, body)
	if f.uploadPartOut != nil {
		return f.uploadPartOut, f.uploadPartErr
	}
	return &s3.UploadPartOutput{ETag: aws.String("etag-" + aws.ToString(in.UploadId))}, f.uploadPartErr
}

func (f *fakeS3Client) CompleteMultipartUpload(_ context.Context, in *s3.CompleteMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	f.completeInputs = append(f.completeInputs, in)
	return &s3.CompleteMultipartUploadOutput{}, f.completeMultipartEr
}

func (f *fakeS3Client) AbortMultipartUpload(_ context.Context, in *s3.AbortMultipartUploadInput, _ ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	f.abortInputs = append(f.abortInputs, in)
	return &s3.AbortMultipartUploadOutput{}, f.abortMultipartErr
}

func (f *fakeS3Client) HeadBucket(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, nil
}

func (f *fakeS3Client) CreateBucket(_ context.Context, _ *s3.CreateBucketInput, _ ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	return &s3.CreateBucketOutput{}, nil
}

func (f *fakeS3Client) PutBucketTagging(_ context.Context, _ *s3.PutBucketTaggingInput, _ ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
	return &s3.PutBucketTaggingOutput{}, nil
}

func (f *fakeS3Client) DeleteObject(_ context.Context, _ *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return &s3.DeleteObjectOutput{}, nil
}

// chunkPlan is a small fixture helper building a synthetic plan of n
// one-byte chunks, for driving multipartUpload without huge files.
func chunkPlan(n int) []uploadChunk {
	chunks := make([]uploadChunk, n)
	for i := range chunks {
		chunks[i] = uploadChunk{offset: int64(i), size: 1}
	}
	return chunks
}

func TestNeedsMultipartUpload(t *testing.T) {
	gb := int64(1024 * 1024 * 1024)
	tests := []struct {
		name string
		size int64
		want bool
	}{
		{"small archive stays single-put", 64 * 1024 * 1024, false},
		{"exactly 5 GB still fits one PutObject", 5 * gb, false},
		{"one byte over the limit needs multipart", 5*gb + 1, true},
		{"huge archive needs multipart", 500 * gb, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsMultipartUpload(tt.size); got != tt.want {
				t.Errorf("needsMultipartUpload(%d) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

func TestUploadChunkPlan(t *testing.T) {
	mb := int64(1024 * 1024)
	tests := []struct {
		name      string
		size      int64
		chunkSize int64
		want      []uploadChunk
	}{
		{"empty size yields no chunks", 0, multipartChunkSize, nil},
		{"one chunk when size is small", 1024, multipartChunkSize, []uploadChunk{{offset: 0, size: 1024}}},
		{"exact multiple splits evenly", 2 * mb, mb, []uploadChunk{{offset: 0, size: mb}, {offset: mb, size: mb}}},
		{"remainder is the smaller last chunk", 2*mb + 3, mb, []uploadChunk{{offset: 0, size: mb}, {offset: mb, size: mb}, {offset: 2 * mb, size: 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uploadChunkPlan(tt.size, tt.chunkSize)
			if len(got) != len(tt.want) {
				t.Fatalf("uploadChunkPlan(%d,%d) = %v, want %v", tt.size, tt.chunkSize, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("uploadChunkPlan(%d,%d)[%d] = %+v, want %+v", tt.size, tt.chunkSize, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// writeArchive creates a temp file with the given content and returns its path.
func writeArchive(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "build.zip")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing archive: %v", err)
	}
	return path
}

func TestUploadToS3UsesSinglePutBelowLimit(t *testing.T) {
	s3c := &fakeS3Client{}
	d := &Deployer{s3Client: s3c}

	if err := d.uploadToS3(context.Background(), "bucket", "key.zip", writeArchive(t, "small")); err != nil {
		t.Fatalf("uploadToS3() error = %v", err)
	}
	if s3c.putCalls != 1 || s3c.createMultipartCalls != 0 {
		t.Errorf("put=%d multipart=%d, want 1/0 for a small archive", s3c.putCalls, s3c.createMultipartCalls)
	}
}

func TestUploadToS3WrapsPutError(t *testing.T) {
	d := &Deployer{s3Client: &fakeS3Client{putObjectErr: errors.New("boom")}}

	err := d.uploadToS3(context.Background(), "bucket", "key.zip", writeArchive(t, "small"))
	if err == nil || !strings.Contains(err.Error(), "uploading to S3") {
		t.Fatalf("uploadToS3() error = %v, want uploading to S3 wrap", err)
	}
}

func TestMultipartUploadSuccess(t *testing.T) {
	s3c := &fakeS3Client{}
	d := &Deployer{s3Client: s3c}

	file, err := os.Open(writeArchive(t, "abc"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := d.multipartUpload(context.Background(), "bucket", "key.zip", file, chunkPlan(3)); err != nil {
		t.Fatalf("multipartUpload() error = %v", err)
	}
	verifyMultipartCallShape(t, s3c)
	verifyMultipartParts(t, s3c)
}

// verifyMultipartCallShape checks the create/upload/complete/abort call counts
// of a successful three-part upload.
func verifyMultipartCallShape(t *testing.T, s3c *fakeS3Client) {
	t.Helper()
	if s3c.createMultipartCalls != 1 || len(s3c.uploadPartInputs) != 3 || len(s3c.completeInputs) != 1 {
		t.Errorf("calls create=%d parts=%d complete=%d, want 1/3/1",
			s3c.createMultipartCalls, len(s3c.uploadPartInputs), len(s3c.completeInputs))
	}
	if len(s3c.abortInputs) != 0 {
		t.Errorf("abortInputs = %d, want 0 on success", len(s3c.abortInputs))
	}
}

// verifyMultipartParts checks per-part numbers, bodies, upload IDs, and the
// completed part list handed to CompleteMultipartUpload.
func verifyMultipartParts(t *testing.T, s3c *fakeS3Client) {
	t.Helper()
	for i := range s3c.uploadPartInputs {
		if aws.ToInt32(s3c.uploadPartInputs[i].PartNumber) != int32(i+1) {
			t.Errorf("part %d PartNumber = %d, want %d", i, aws.ToInt32(s3c.uploadPartInputs[i].PartNumber), i+1)
		}
		if string(s3c.uploadPartBodies[i]) != string(rune('a'+i)) {
			t.Errorf("part %d body = %q, want the byte at offset %d", i, s3c.uploadPartBodies[i], i)
		}
		if aws.ToString(s3c.uploadPartInputs[i].UploadId) != "upload-1" {
			t.Errorf("part %d UploadId = %q, want upload-1", i, aws.ToString(s3c.uploadPartInputs[i].UploadId))
		}
	}
	parts := s3c.completeInputs[0].MultipartUpload.Parts
	if len(parts) != 3 || aws.ToInt32(parts[2].PartNumber) != 3 {
		t.Errorf("completed parts = %+v, want 3 in order", parts)
	}
}

func TestMultipartUploadAbortsOnPartFailure(t *testing.T) {
	s3c := &fakeS3Client{uploadPartErr: errors.New("boom")}
	d := &Deployer{s3Client: s3c}

	file, err := os.Open(writeArchive(t, "ab"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	err = d.multipartUpload(context.Background(), "bucket", "key.zip", file, chunkPlan(2))
	if err == nil || !strings.Contains(err.Error(), "uploading part 1") {
		t.Fatalf("multipartUpload() error = %v, want part upload failure", err)
	}
	if len(s3c.abortInputs) != 1 || aws.ToString(s3c.abortInputs[0].UploadId) != "upload-1" {
		t.Fatalf("abortInputs = %+v, want one abort of upload-1", s3c.abortInputs)
	}
	if len(s3c.completeInputs) != 0 {
		t.Errorf("completeInputs = %d, want 0 after a part failure", len(s3c.completeInputs))
	}
}

func TestMultipartUploadAbortsOnCompleteFailure(t *testing.T) {
	s3c := &fakeS3Client{completeMultipartEr: errors.New("boom")}
	d := &Deployer{s3Client: s3c}

	file, err := os.Open(writeArchive(t, "ab"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	err = d.multipartUpload(context.Background(), "bucket", "key.zip", file, chunkPlan(2))
	if err == nil || !strings.Contains(err.Error(), "completing multipart upload") {
		t.Fatalf("multipartUpload() error = %v, want completion failure", err)
	}
	if len(s3c.abortInputs) != 1 {
		t.Errorf("abortInputs = %d, want 1 after a completion failure", len(s3c.abortInputs))
	}
}

func TestMultipartUploadCreateFailure(t *testing.T) {
	s3c := &fakeS3Client{createMultipartErr: errors.New("boom")}
	d := &Deployer{s3Client: s3c}

	err := d.multipartUpload(context.Background(), "bucket", "key.zip", &bytes.Reader{}, chunkPlan(1))
	if err == nil || !strings.Contains(err.Error(), "starting multipart upload") {
		t.Fatalf("multipartUpload() error = %v, want creation failure", err)
	}
	if len(s3c.uploadPartInputs) != 0 || len(s3c.abortInputs) != 0 {
		t.Errorf("parts=%d aborts=%d, want 0/0 without an upload ID", len(s3c.uploadPartInputs), len(s3c.abortInputs))
	}
}

// TestMultipartUploadAbortSurvivesCancellation pins the cleanup contract: the
// incomplete upload is aborted with a context that is detached from the
// caller's cancellation, so an interrupted run still removes uploaded parts.
func TestMultipartUploadAbortSurvivesCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	s3c := &fakeS3Client{uploadPartErr: errors.New("boom")}
	d := &Deployer{s3Client: s3c}

	file, err := os.Open(writeArchive(t, "ab"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := d.multipartUpload(canceled, "bucket", "key.zip", file, chunkPlan(2)); err == nil {
		t.Fatal("multipartUpload() error = nil, want failure")
	}
	if len(s3c.abortInputs) != 1 {
		t.Errorf("abortInputs = %d, want the abort to run despite the canceled context", len(s3c.abortInputs))
	}
}

// TestMultipartUploadAbortFailureIsNonFatal pins the best-effort semantics of
// the cleanup: a failed abort is reported but does not mask the upload error.
func TestMultipartUploadAbortFailureIsNonFatal(t *testing.T) {
	s3c := &fakeS3Client{uploadPartErr: errors.New("boom"), abortMultipartErr: errors.New("abort boom")}
	d := &Deployer{s3Client: s3c}

	file, err := os.Open(writeArchive(t, "ab"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	err = d.multipartUpload(context.Background(), "bucket", "key.zip", file, chunkPlan(2))
	if err == nil || !strings.Contains(err.Error(), "uploading part 1") {
		t.Fatalf("multipartUpload() error = %v, want the part failure, not the abort failure", err)
	}
	if len(s3c.abortInputs) != 1 {
		t.Errorf("abortInputs = %d, want the abort attempted", len(s3c.abortInputs))
	}
}

// compile-time assertion that the production client satisfies the seam.
var _ s3API = (*s3.Client)(nil)
