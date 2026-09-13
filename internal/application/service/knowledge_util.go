package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	filesvc "github.com/Tencent/WeKnora/internal/application/service/file"
	werrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// unknownFileType is returned by getFileType when a name carries no extension.
const unknownFileType = "unknown"

// supportedImportFileExtensions is the single source of truth for extensions
// accepted by every knowledge import path: direct upload, file-URL download,
// and the worker's post-download re-check. Keeping one set avoids the drift
// that let direct upload accept xlsx while URL import rejected it (#2447).
var supportedImportFileExtensions = map[string]struct{}{
	"pdf": {}, "txt": {}, "docx": {}, "doc": {}, "epub": {},
	"html": {}, "htm": {}, "mhtml": {}, "md": {}, "markdown": {},
	"png": {}, "jpg": {}, "jpeg": {}, "gif": {},
	"csv": {}, "xlsx": {}, "xls": {}, "pptx": {}, "ppt": {}, "json": {},
	"mp3": {}, "wav": {}, "m4a": {}, "flac": {}, "ogg": {},
}

// dataTableFileExtensions are the tabular formats supported by the structured
// SQL analysis path. Legacy XLS remains a supported document import, but
// DuckDB's read_xlsx reader cannot load the binary XLS format.
var dataTableFileExtensions = map[string]struct{}{
	"csv": {}, "xlsx": {},
}

// normalizeFileExtension lowercases an extension and strips a leading dot so
// callers can pass either "xlsx", ".XLSX", or a raw user-supplied file_type.
func normalizeFileExtension(ext string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
}

// isSupportedImportExtension reports whether a bare extension can be imported.
func isSupportedImportExtension(ext string) bool {
	ext = normalizeFileExtension(ext)
	if ext == "" || ext == unknownFileType {
		return false
	}
	_, ok := supportedImportFileExtensions[ext]
	return ok
}

// isValidFileType checks if a filename's extension is supported for import.
func isValidFileType(filename string) bool {
	return isSupportedImportExtension(getFileType(filename))
}

// isDataTableFileType reports whether an extension supports structured SQL
// analysis and therefore gets an extra table-summary task.
func isDataTableFileType(ext string) bool {
	_, ok := dataTableFileExtensions[normalizeFileExtension(ext)]
	return ok
}

// validateImportFileType applies the extension constraints shared by every
// file import path and reports a user-facing reason when one is violated.
func validateImportFileType(fileType string) error {
	fileType = normalizeFileExtension(fileType)
	if fileType == "" || fileType == unknownFileType {
		return werrors.NewBadRequestError("无法确定文件类型")
	}
	if IsVideoType(fileType) {
		return werrors.NewBadRequestError("暂不支持上传视频文件")
	}
	if !isSupportedImportExtension(fileType) {
		return werrors.NewBadRequestError(fmt.Sprintf("不支持的文件类型: %s", fileType))
	}
	return nil
}

// getFileType extracts the file extension from a filename
func getFileType(filename string) string {
	ext := strings.Split(filename, ".")
	if len(ext) < 2 {
		return unknownFileType
	}
	return ext[len(ext)-1]
}

// isValidURL verifies if a URL is valid
// isValidURL 检查URL是否有效
func isValidURL(url string) bool {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return true
	}
	return false
}

// calculateFileHash calculates MD5 hash of a file
func calculateFileHash(file *multipart.FileHeader) (string, error) {
	f, err := file.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	// Reset file pointer for subsequent operations
	if _, err := f.Seek(0, 0); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func calculateStr(strList ...string) string {
	h := md5.New()
	input := strings.Join(strList, "")
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *knowledgeService) getVLMConfig(ctx context.Context, kb *types.KnowledgeBase) (*types.DocParserVLMConfig, error) {
	if kb == nil {
		return nil, nil
	}
	// 兼容老版本：直接使用 ModelName 和 BaseURL
	if kb.VLMConfig.ModelName != "" && kb.VLMConfig.BaseURL != "" {
		return &types.DocParserVLMConfig{
			ModelName:     kb.VLMConfig.ModelName,
			BaseURL:       kb.VLMConfig.BaseURL,
			APIKey:        kb.VLMConfig.APIKey,
			InterfaceType: kb.VLMConfig.InterfaceType,
		}, nil
	}

	// 新版本：未启用或无模型ID时返回nil
	if !kb.VLMConfig.Enabled || kb.VLMConfig.ModelID == "" {
		return nil, nil
	}

	model, err := s.modelService.GetModelByID(ctx, kb.VLMConfig.ModelID)
	if err != nil {
		return nil, err
	}

	interfaceType := model.Parameters.InterfaceType
	if interfaceType == "" {
		interfaceType = "openai"
	}

	return &types.DocParserVLMConfig{
		ModelName:     model.Name,
		BaseURL:       model.Parameters.BaseURL,
		APIKey:        model.Parameters.APIKey,
		InterfaceType: interfaceType,
	}, nil
}

// resolveFileService resolves the KB's explicit global backend, or the global
// default when the binding is unset. A failed lookup remains a hard error on
// the next file operation; it never selects tenant or environment config.
func (s *knowledgeService) resolveFileService(ctx context.Context, kb *types.KnowledgeBase) interfaces.FileService {
	if kb == nil {
		return filesvc.NewUnavailableFileService(fmt.Errorf("knowledge base is required to resolve storage"))
	}

	backendID := ""
	if kb.StorageBackendID != nil {
		backendID = strings.TrimSpace(*kb.StorageBackendID)
	}
	if s.storageResolver != nil {
		baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
		svc, resolvedProvider, err := s.storageResolver.ResolveFileService(ctx, backendID, baseDir)
		if err == nil && svc != nil {
			logger.Infof(ctx, "[storage] resolveFileService selected instance: kb=%s backend=%s provider=%s", kb.ID, backendID, resolvedProvider)
			return svc
		}
		if err != nil {
			logger.Errorf(ctx, "Failed to resolve storage backend for kb=%s: %v", kb.ID, err)
			return filesvc.NewUnavailableFileService(err)
		}
	}
	return filesvc.NewUnavailableFileService(fmt.Errorf("storage backend resolver is not configured"))
}

// resolveFileServiceForPath resolves the backend carried by a resource/storage
// reference, or the KB binding/global default for an unqualified provider ref.
// The provider scheme must match the resolved backend.
func (s *knowledgeService) resolveFileServiceForPath(ctx context.Context, kb *types.KnowledgeBase, filePath string) interfaces.FileService {
	// A resource:// reference belongs to the tenant that registered it. Shared
	// KB requests use the viewer's effective tenant in ctx, which can otherwise
	// select the wrong storage backend and pass the resource URL to local disk.
	if _, ok := types.ParseResourcePath(filePath); ok && s.resourceCatalog != nil && s.storageResolver != nil {
		resource, err := s.resourceCatalog.Resolve(ctx, filePath)
		if err == nil && resource != nil {
			baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
			if resolved, resolvedProvider, resolveErr := s.storageResolver.ResolveFileService(ctx, resource.StorageBackendID, baseDir); resolveErr == nil && resolved != nil {
				if resource.Provider != "" && !strings.EqualFold(resource.Provider, resolvedProvider) {
					return filesvc.NewUnavailableFileService(fmt.Errorf("resource provider %q does not match backend provider %q", resource.Provider, resolvedProvider))
				}
				return resolved
			} else if resolveErr != nil {
				logger.Warnf(ctx, "[storage] failed to resolve resource backend: resource=%s tenant=%d err=%v", resource.Handle, resource.TenantID, resolveErr)
				return filesvc.NewUnavailableFileService(resolveErr)
			}
		} else if err != nil {
			return filesvc.NewUnavailableFileService(err)
		} else {
			return filesvc.NewUnavailableFileService(fmt.Errorf("resource reference was not found"))
		}
	}

	if backendID, inner, ok := types.ParseStorageBackendPath(filePath); ok && s.storageResolver != nil {
		baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
		if resolved, resolvedProvider, err := s.storageResolver.ResolveFileService(ctx, backendID, baseDir); err == nil {
			if provider := types.ParseProviderScheme(inner); provider != "" && !strings.EqualFold(provider, resolvedProvider) {
				return filesvc.NewUnavailableFileService(fmt.Errorf("stored file provider %q does not match backend provider %q", provider, resolvedProvider))
			}
			return resolved
		} else {
			logger.Warnf(ctx, "[storage] failed to resolve backend from file path: backend=%s err=%v", backendID, err)
			return filesvc.NewUnavailableFileService(err)
		}
	}
	if filePath == "" {
		return s.resolveFileService(ctx, kb)
	}

	inferred := types.InferStorageFromFilePath(filePath)
	if inferred == "" {
		return s.resolveFileService(ctx, kb)
	}
	backendID := ""
	if kb != nil && kb.StorageBackendID != nil {
		backendID = strings.TrimSpace(*kb.StorageBackendID)
	}
	if s.storageResolver == nil {
		return filesvc.NewUnavailableFileService(fmt.Errorf("storage backend resolver is not configured"))
	}
	baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
	svc, resolvedProvider, err := s.storageResolver.ResolveFileService(ctx, backendID, baseDir)
	if err != nil {
		return filesvc.NewUnavailableFileService(err)
	}
	if !strings.EqualFold(inferred, resolvedProvider) {
		return filesvc.NewUnavailableFileService(fmt.Errorf("stored file provider %q does not match backend provider %q", inferred, resolvedProvider))
	}
	return svc
}

func IsImageType(fileType string) bool {
	switch fileType {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "tiff":
		return true
	default:
		return false
	}
}

// IsAudioType checks if a file type is an audio format
func IsAudioType(fileType string) bool {
	switch strings.ToLower(fileType) {
	case "mp3", "wav", "m4a", "flac", "ogg":
		return true
	default:
		return false
	}
}

// IsVideoType checks if a file type is a video format
func IsVideoType(fileType string) bool {
	switch strings.ToLower(fileType) {
	case "mp4", "mov", "avi", "mkv", "webm", "wmv", "flv":
		return true
	default:
		return false
	}
}

// downloadFileFromURL downloads a remote file to a temp file and returns its binary content.
// payloadFileName and payloadFileType are in/out pointers: if they point to an empty string,
// the function resolves the value from Content-Disposition / URL path and writes it back.
// It does NOT perform SSRF validation — callers are responsible for that.
func downloadFileFromURL(ctx context.Context, fileURL string, payloadFileName, payloadFileType *string) ([]byte, error) {
	httpClient := secutils.NewSSRFSafeHTTPClient(secutils.SSRFSafeHTTPClientConfig{
		Timeout:      60 * time.Second,
		MaxRedirects: 10,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for file URL: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote server returned status %d", resp.StatusCode)
	}

	// Reject oversized files early via Content-Length
	if contentLength := resp.ContentLength; contentLength > maxFileURLSize {
		return nil, fmt.Errorf("file size %d bytes exceeds limit of %d bytes (10MB)", contentLength, maxFileURLSize)
	}

	// Resolve fileName: payload > Content-Disposition > URL path
	if *payloadFileName == "" {
		if cd := resp.Header.Get("Content-Disposition"); cd != "" {
			*payloadFileName = extractFileNameFromContentDisposition(cd)
		}
	}
	if *payloadFileName == "" {
		*payloadFileName = extractFileNameFromURL(fileURL)
	}
	if *payloadFileType == "" && *payloadFileName != "" {
		*payloadFileType = getFileType(*payloadFileName)
	}

	// Stream response body into a temp file, capped at maxFileURLSize
	tmpFile, err := os.CreateTemp("", "weknora-fileurl-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	limiter := &io.LimitedReader{R: resp.Body, N: maxFileURLSize + 1}
	written, err := io.Copy(tmpFile, limiter)
	tmpFile.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	if written > maxFileURLSize {
		return nil, fmt.Errorf("file size exceeds limit of 10MB")
	}

	contentBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp file: %w", err)
	}

	return contentBytes, nil
}
