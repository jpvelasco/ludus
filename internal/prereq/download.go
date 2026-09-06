package prereq

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

// downloadFile downloads a URL to a local file with progress reporting.
// The write goes to a .partial sibling that is removed on any failure and
// renamed into place only on success, so an interrupted transfer never leaves
// a truncated file at the final path — the toolchain cache treats any file
// there as a complete installer.
func downloadFile(dst string, url string) error {
	resp, err := http.Get(url) //nolint:gosec // URL is from our hardcoded toolchain map, not user input
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	partial := dst + ".partial"
	out, err := os.Create(partial)
	if err != nil {
		return err
	}

	copyErr := copyWithProgress(resp.Body, out, resp.ContentLength)
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(partial)
		return err
	}
	return os.Rename(partial, dst)
}

// copyWithProgress copies from reader to writer, printing progress at 10% intervals.
func copyWithProgress(src io.Reader, dst io.Writer, totalBytes int64) error {
	buf := make([]byte, 32*1024)
	var downloaded int64
	lastPct := -1

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)
			if totalBytes > 0 {
				pct := int(downloaded * 100 / totalBytes)
				if pct/10 > lastPct/10 {
					fmt.Printf("  Progress: %d%% (%d / %d MB)\n", pct, downloaded/(1024*1024), totalBytes/(1024*1024))
					lastPct = pct
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}
