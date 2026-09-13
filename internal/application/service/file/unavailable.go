package file

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// unavailableFileService keeps a backend-resolution failure attached to the
// next concrete file operation. Several older service helpers return only a
// FileService, so this preserves their signatures while preventing a failed
// explicit/global lookup from silently using an environment-backed service.
type unavailableFileService struct {
	err error
}

func NewUnavailableFileService(err error) interfaces.FileService {
	if err == nil {
		err = fmt.Errorf("storage backend is unavailable")
	}
	return &unavailableFileService{err: err}
}

func (s *unavailableFileService) CheckConnectivity(context.Context) error { return s.err }
func (s *unavailableFileService) SaveFile(context.Context, *multipart.FileHeader, uint64, string) (string, error) {
	return "", s.err
}
func (s *unavailableFileService) SaveBytes(context.Context, []byte, uint64, string, bool) (string, error) {
	return "", s.err
}
func (s *unavailableFileService) GetFile(context.Context, string) (io.ReadCloser, error) {
	return nil, s.err
}
func (s *unavailableFileService) GetFileURL(context.Context, string) (string, error) {
	return "", s.err
}
func (s *unavailableFileService) DeleteFile(context.Context, string) error { return s.err }
func (s *unavailableFileService) CopyFile(context.Context, string, uint64, string) (string, error) {
	return "", s.err
}

var _ interfaces.FileService = (*unavailableFileService)(nil)
