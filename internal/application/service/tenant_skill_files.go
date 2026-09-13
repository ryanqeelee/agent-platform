package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"unicode/utf8"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
)

// skillFileTextLimit is how much of a text file the admin browser is given.
// The archive itself may hold up to maxSkillBundleFileBytes; dumping that
// into a JSON response would freeze the settings drawer.
const skillFileTextLimit = 1 << 20 // 1 MiB

// skillFileImageLimit is the decoded size cap for an inline image preview.
const skillFileImageLimit = 2 << 20 // 2 MiB

const (
	skillFileEncodingUTF8   = "utf-8"
	skillFileEncodingBase64 = "base64"
	skillFileEncodingBinary = "binary"

	// The settings drawer lists the tree then immediately opens SKILL.md, and
	// every later click is another read of the same zip. Keep a modest
	// process-wide budget: 512 MiB is the install/decompress cap, not RAM
	// this cache is allowed to pin. A zip larger than the budget is still
	// cached so a large skill's drawer does not re-download on every click,
	// but it is the only occupant until something else is opened.
	skillBundleArchiveCacheSlots = 8
	skillBundleArchiveCacheBytes = 64 << 20
)

// SkillFileEntry is one path in an installed skill's stored archive.
type SkillFileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// SkillFileContent is one file the admin browser asked to open.
type SkillFileContent struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Encoding  string `json:"encoding"`
	Content   string `json:"content,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
}

// ListSkillFiles lists the stored archive of one installed skill. The files
// come from the uploaded bundle rather than the live image: browsing must
// work while the skill is still installing, and without booting a sandbox.
func (s *TenantSkillService) ListSkillFiles(
	ctx context.Context, configID, skillID string,
) ([]SkillFileEntry, error) {
	archive, err := s.skillBundleArchive(ctx, configID, skillID)
	if err != nil {
		return nil, err
	}
	return listSkillZipFiles(archive)
}

// ReadSkillFile returns one file from the stored archive. Binary files are
// either inlined as base64 (images small enough to preview) or reported
// without a body so the UI can say they cannot be opened.
func (s *TenantSkillService) ReadSkillFile(
	ctx context.Context, configID, skillID, relativePath string,
) (*SkillFileContent, error) {
	clean, err := safeSkillFilePath(relativePath)
	if err != nil {
		return nil, apperrors.NewBadRequestError(err.Error())
	}
	archive, err := s.skillBundleArchive(ctx, configID, skillID)
	if err != nil {
		return nil, err
	}
	body, err := readSkillZipFile(archive, clean)
	if err != nil {
		if errors.Is(err, errSkillFileMissing) {
			return nil, apperrors.NewNotFoundError("skill file not found")
		}
		return nil, err
	}
	return projectSkillFileContent(clean, body), nil
}

func (s *TenantSkillService) skillBundleArchive(
	ctx context.Context, configID, skillID string,
) ([]byte, error) {
	skill, err := s.skills.GetSkill(ctx, configID, skillID)
	if err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, apperrors.NewNotFoundError("skill not found")
	}
	if strings.TrimSpace(skill.CatalogID) == "" || strings.TrimSpace(skill.BundleSHA256) == "" {
		return nil, apperrors.NewNotFoundError("skill files are not available")
	}
	catalog, err := s.resolveCatalog(ctx, skill.CatalogID)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(skill.BundleSHA256)
	if strings.TrimSpace(catalog.BundleSHA256) != key {
		return nil, apperrors.NewNotFoundError("skill files are not available")
	}
	if cached := s.cachedSkillBundle(key); cached != nil {
		return cached, nil
	}
	v, err, _ := s.bundleLoad.Do(key, func() (interface{}, error) {
		if cached := s.cachedSkillBundle(key); cached != nil {
			return cached, nil
		}
		archive, err := s.catalogBundleArchive(ctx, catalog)
		if err != nil {
			return nil, err
		}
		if !archiveMatchesSHA(archive, key) {
			return nil, apperrors.NewNotFoundError("skill files are not available")
		}
		s.storeSkillBundle(key, archive)
		return archive, nil
	})
	if err != nil {
		return nil, err
	}
	loaded, ok := v.([]byte)
	if !ok || len(loaded) == 0 {
		return nil, apperrors.NewNotFoundError("skill files are not available")
	}
	return loaded, nil
}

func (s *TenantSkillService) cachedSkillBundle(key string) []byte {
	if s == nil || s.bundleCache == nil {
		return nil
	}
	return s.bundleCache.get(key)
}

func (s *TenantSkillService) storeSkillBundle(key string, archive []byte) {
	if s == nil || s.bundleCache == nil {
		return
	}
	s.bundleCache.put(key, archive)
}

type skillBundleArchiveCache struct {
	mu       sync.Mutex
	entries  []cachedSkillArchive
	slots    int
	maxBytes int
}

type cachedSkillArchive struct {
	key     string
	archive []byte
}

func newSkillBundleArchiveCache() *skillBundleArchiveCache {
	return &skillBundleArchiveCache{
		slots:    skillBundleArchiveCacheSlots,
		maxBytes: skillBundleArchiveCacheBytes,
	}
}

func (c *skillBundleArchiveCache) get(key string) []byte {
	if c == nil || key == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, entry := range c.entries {
		if entry.key != key {
			continue
		}
		c.entries = append(c.entries[:i], c.entries[i+1:]...)
		c.entries = append([]cachedSkillArchive{entry}, c.entries...)
		return entry.archive
	}
	return nil
}

func (c *skillBundleArchiveCache) put(key string, archive []byte) {
	if c == nil || key == "" || len(archive) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, entry := range c.entries {
		if entry.key != key {
			continue
		}
		c.entries = append(c.entries[:i], c.entries[i+1:]...)
		break
	}
	c.entries = append([]cachedSkillArchive{{key: key, archive: archive}}, c.entries...)
	slots, maxBytes := c.slots, c.maxBytes
	if slots <= 0 {
		slots = skillBundleArchiveCacheSlots
	}
	if maxBytes <= 0 {
		maxBytes = skillBundleArchiveCacheBytes
	}
	for len(c.entries) > 1 && (len(c.entries) > slots || c.cachedBytes() > maxBytes) {
		c.entries = c.entries[:len(c.entries)-1]
	}
}

func (c *skillBundleArchiveCache) cachedBytes() int {
	total := 0
	for _, entry := range c.entries {
		total += len(entry.archive)
	}
	return total
}

// safeSkillFilePath normalises a caller-supplied relative path and refuses
// anything that leaves the skill directory.
func safeSkillFilePath(relativePath string) (string, error) {
	trimmed := strings.TrimSpace(relativePath)
	if trimmed == "" {
		return "", fmt.Errorf("skill file path is required")
	}
	if strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("invalid skill file path: %s", relativePath)
	}
	if path.IsAbs(trimmed) {
		return "", fmt.Errorf("invalid skill file path: %s", relativePath)
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid skill file path: %s", relativePath)
		}
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid skill file path: %s", relativePath)
	}
	return clean, nil
}

func projectSkillFileContent(rel string, body []byte) *SkillFileContent {
	out := &SkillFileContent{
		Path: rel,
		Size: int64(len(body)),
	}
	if mediaType, ok := skillImageMediaType(rel); ok {
		out.MediaType = mediaType
		if len(body) > skillFileImageLimit {
			out.Encoding = skillFileEncodingBinary
			out.Binary = true
			return out
		}
		out.Encoding = skillFileEncodingBase64
		out.Content = base64.StdEncoding.EncodeToString(body)
		return out
	}
	if skillFileLooksBinary(body) {
		out.Encoding = skillFileEncodingBinary
		out.Binary = true
		return out
	}
	out.Encoding = skillFileEncodingUTF8
	if ext := strings.ToLower(path.Ext(rel)); ext != "" {
		out.MediaType = "text/plain"
		if ext == ".md" || ext == ".markdown" {
			out.MediaType = "text/markdown"
		}
	}
	if len(body) > skillFileTextLimit {
		out.Content = string(body[:skillFileTextLimit])
		out.Truncated = true
		return out
	}
	out.Content = string(body)
	return out
}

func skillFileLooksBinary(body []byte) bool {
	if bytes.IndexByte(body, 0) >= 0 {
		return true
	}
	return !utf8.Valid(body)
}

func skillImageMediaType(rel string) (string, bool) {
	switch strings.ToLower(path.Ext(rel)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	case ".bmp":
		return "image/bmp", true
	case ".ico":
		return "image/x-icon", true
	case ".svg":
		return "image/svg+xml", true
	default:
		return "", false
	}
}
