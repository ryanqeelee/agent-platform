package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPlatformParserConfigRepositorySingletonRoundTrip(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE platform_parser_config (
		id INTEGER PRIMARY KEY CHECK (id = 1), config TEXT NOT NULL,
		updated_by TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
	)`).Error)
	repo := NewPlatformParserConfigRepository(db)

	missing, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.Nil(t, missing)

	now := time.Unix(1_700_000_000, 0).UTC()
	first, err := repo.Upsert(context.Background(), &types.ParserEngineConfig{
		MinerUAPIKey: "secret-one", MinerUEndpoint: "https://mineru.example.invalid",
	}, "admin-one", now)
	require.NoError(t, err)
	require.Equal(t, uint8(1), first.ID)

	var raw string
	require.NoError(t, db.Raw("SELECT config FROM platform_parser_config WHERE id = 1").Scan(&raw).Error)
	require.NotContains(t, raw, "secret-one")
	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	require.True(t, strings.HasPrefix(stored["mineru_api_key"].(string), "enc:v1:"))

	loaded, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, "secret-one", loaded.Config.MinerUAPIKey)

	_, err = repo.Upsert(context.Background(), &types.ParserEngineConfig{MinerUAPIKey: "secret-two"}, "admin-two", now.Add(time.Hour))
	require.NoError(t, err)
	loaded, err = repo.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, "secret-two", loaded.Config.MinerUAPIKey)
	require.Equal(t, "admin-two", loaded.UpdatedBy)
	require.True(t, loaded.CreatedAt.Equal(now), "upsert must retain singleton creation metadata")
	require.True(t, loaded.UpdatedAt.Equal(now.Add(time.Hour)))
	var count int64
	require.NoError(t, db.Table("platform_parser_config").Count(&count).Error)
	require.Equal(t, int64(1), count)

	t.Setenv("SYSTEM_AES_KEY", "")
	_, err = repo.Upsert(context.Background(), &types.ParserEngineConfig{MinerUAPIKey: "must-not-persist"}, "admin-three", now.Add(2*time.Hour))
	require.ErrorContains(t, err, "SYSTEM_AES_KEY")
}
