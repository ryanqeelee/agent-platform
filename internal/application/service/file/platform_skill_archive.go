package file

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

type platformSkillArchiveDriver interface {
	putPlatformSkillArchive(ctx context.Context, key string, data []byte) (string, error)
	platformSkillArchiveKey(ref string) (string, error)
}

func PutPlatformSkillArchive(ctx context.Context, inner interfaces.FileService, key string, data []byte) (string, error) {
	key, err := canonicalPlatformSkillKey(key)
	if err != nil {
		return "", err
	}
	driver, ok := inner.(platformSkillArchiveDriver)
	if !ok {
		return "", fmt.Errorf("platform skill archives are not supported by this storage driver")
	}
	return driver.putPlatformSkillArchive(ctx, key, data)
}

func ValidatePlatformSkillArchiveRef(inner interfaces.FileService, ref string) error {
	driver, ok := inner.(platformSkillArchiveDriver)
	if !ok {
		return fmt.Errorf("platform skill archives are not supported by this storage driver")
	}
	key, err := driver.platformSkillArchiveKey(ref)
	if err != nil {
		return err
	}
	canonical, err := canonicalPlatformSkillKey(key)
	if err != nil {
		return err
	}
	if canonical != key {
		return fmt.Errorf("platform skill archive reference is not canonical")
	}
	return nil
}

func canonicalPlatformSkillKey(key string) (string, error) {
	canonical := strings.TrimSpace(key)
	if strings.Contains(key, "\\") {
		return "", fmt.Errorf("platform skill object key must use forward slashes")
	}
	decoded, err := url.PathUnescape(canonical)
	if err != nil || decoded != canonical {
		return "", fmt.Errorf("platform skill object key must not contain URL-encoded path segments")
	}
	if err := secutils.SafeObjectKey(canonical); err != nil {
		return "", fmt.Errorf("invalid platform skill object key: %w", err)
	}
	if !strings.HasPrefix(canonical, "platform/skills/") || canonical == "platform/skills/" {
		return "", fmt.Errorf("object key must be under platform/skills/")
	}
	if strings.HasPrefix(canonical, "/") || path.Clean(canonical) != canonical {
		return "", fmt.Errorf("platform skill object key must be canonical")
	}
	return canonical, nil
}

func platformSkillLogicalKey(objectKey, configuredPrefix string) (string, error) {
	if strings.TrimSpace(objectKey) != objectKey || strings.Contains(objectKey, "\\") {
		return "", fmt.Errorf("platform skill archive reference is not canonical")
	}
	decoded, err := url.PathUnescape(objectKey)
	if err != nil || decoded != objectKey {
		return "", fmt.Errorf("platform skill archive reference must not contain URL-encoded path segments")
	}
	if err := secutils.SafeObjectKey(objectKey); err != nil {
		return "", fmt.Errorf("invalid platform skill archive reference: %w", err)
	}
	if strings.HasPrefix(objectKey, "/") || path.Clean(objectKey) != objectKey {
		return "", fmt.Errorf("platform skill archive reference is not canonical")
	}
	prefix := strings.Trim(strings.ReplaceAll(configuredPrefix, "\\", "/"), "/")
	if prefix == "" {
		return objectKey, nil
	}
	prefix += "/"
	if !strings.HasPrefix(objectKey, prefix) {
		return "", fmt.Errorf("platform skill archive reference is outside the backend prefix")
	}
	return strings.TrimPrefix(objectKey, prefix), nil
}
