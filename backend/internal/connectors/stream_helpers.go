package connectors

import (
	"context"
	"io"
	"os"
)

type tempFileReadCloser struct {
	file *os.File
	path string
}

func (reader *tempFileReadCloser) Read(p []byte) (int, error) {
	return reader.file.Read(p)
}

func (reader *tempFileReadCloser) Close() error {
	closeErr := reader.file.Close()
	removeErr := os.Remove(reader.path)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func openReadStreamFromCopyOut(ctx context.Context, connector interface {
	CopyOut(context.Context, string, io.Writer) error
}, path string) (*tempFileReadCloser, error) {
	tempFile, err := os.CreateTemp("", "mam-connector-read-*")
	if err != nil {
		return nil, err
	}

	tempPath := tempFile.Name()
	tempFile.Close()

	output, err := os.Create(tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, err
	}

	if copyErr := connector.CopyOut(ctx, path, output); copyErr != nil {
		output.Close()
		_ = os.Remove(tempPath)
		return nil, copyErr
	}
	if closeErr := output.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return nil, closeErr
	}

	file, openErr := os.Open(tempPath)
	if openErr != nil {
		_ = os.Remove(tempPath)
		return nil, openErr
	}

	return &tempFileReadCloser{file: file, path: tempPath}, nil
}
