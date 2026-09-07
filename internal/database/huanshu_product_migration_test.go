package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHuanshuProductMigrations(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			driver, dsn := "sqlite3", filepath.Join(t.TempDir(), "migration.db")
			if dialect == "postgres" {
				driver = "postgres"
				dsn = os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
				}
				admin, err := sql.Open(driver, dsn)
				if err != nil {
					t.Fatal(err)
				}
				schema := fmt.Sprintf("huanshu_migration_%d", time.Now().UnixNano())
				if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE"); _ = admin.Close() })
				dsn += " search_path=" + schema
			}
			db, err := sql.Open(driver, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			exec := func(statement string) {
				t.Helper()
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			configType := "TEXT"
			if dialect == "postgres" {
				configType = "JSONB"
			}
			exec(`CREATE TABLE tenant_members (id INTEGER PRIMARY KEY,user_id TEXT NOT NULL,tenant_id INTEGER NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'contributor',status TEXT NOT NULL DEFAULT 'active',invited_by TEXT,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,deleted_at TIMESTAMP,operating_analysis_access BOOLEAN NOT NULL DEFAULT FALSE)`)
			exec(`CREATE UNIQUE INDEX idx_tenant_members_one_active_owner ON tenant_members(tenant_id) WHERE role='owner' AND status='active' AND deleted_at IS NULL`)
			exec(`CREATE TABLE tenant_invitations(id INTEGER PRIMARY KEY,role TEXT,status TEXT)`)
			exec(`CREATE UNIQUE INDEX idx_tenant_members_tenant_id_for_business_roles ON tenant_members(tenant_id,id)`)
			exec(`CREATE TABLE business_role_members(tenant_id INTEGER,role_id TEXT,tenant_member_id INTEGER,
    FOREIGN KEY(tenant_id,tenant_member_id) REFERENCES tenant_members(tenant_id,id) ON DELETE CASCADE)`)
			exec(`CREATE TABLE custom_agents(id TEXT PRIMARY KEY,name TEXT,description TEXT,config ` + configType + `)`)
			exec(`INSERT INTO tenant_members(id,user_id,tenant_id,role,status,operating_analysis_access) VALUES
    (1,'founder',1,'owner','active',TRUE),(2,'finance',1,'contributor','suspended',FALSE),(3,'employee',1,'viewer','active',FALSE)`)
			exec(`INSERT INTO tenant_invitations VALUES(1,'owner','revoked'),(2,'contributor','pending')`)
			exec(`INSERT INTO business_role_members VALUES(1,'finance-role',2)`)
			if dialect == "sqlite" {
				exec(`PRAGMA foreign_keys=ON`)
			}
			exec(`INSERT INTO custom_agents VALUES('custom','WeKnora assistant','Ask WeKnora',
    '{"system_prompt":"You are WeKnora.","intent_prompts":{"rewrite":"Ask WeKnora"},"allowed_tools":["weknora_tool"],"model_id":"WeKnora-model-id"}')`)
			dir, versions := "sqlite", []string{"000020_enterprise_two_roles", "000021_huanshu_agent_brand"}
			if dialect == "postgres" {
				dir = "versioned"
				versions = []string{"000099_enterprise_two_roles", "000100_huanshu_agent_brand"}
			}
			for _, version := range versions {
				data, err := os.ReadFile(filepath.Join("../../migrations", dir, version+".up.sql"))
				if err != nil {
					t.Fatal(err)
				}
				exec(string(data))
			}
			for id, want := range map[int]string{1: "admin", 2: "viewer", 3: "viewer"} {
				var role string
				if err := db.QueryRow(fmt.Sprintf("SELECT role FROM tenant_members WHERE id=%d", id)).Scan(&role); err != nil || role != want {
					t.Fatalf("id=%d role=%s err=%v", id, role, err)
				}
			}
			var status string
			var enabled bool
			var grantCount int
			if err := db.QueryRow("SELECT count(*) FROM business_role_members WHERE tenant_id=1 AND tenant_member_id=2").Scan(&grantCount); err != nil || grantCount != 1 {
				t.Fatal("business-role grants lost", grantCount, err)
			}
			if err := db.QueryRow("SELECT status,operating_analysis_access FROM tenant_members WHERE id=2").Scan(&status, &enabled); err != nil || status != "suspended" || enabled {
				t.Fatalf("membership state changed: %s %v %v", status, enabled, err)
			}
			if err := db.QueryRow("SELECT status FROM tenant_invitations WHERE id=1").Scan(&status); err != nil || status != "revoked" {
				t.Fatal("revoked invitation revived", err)
			}
			exec(`INSERT INTO tenant_members(id,user_id,tenant_id) VALUES(4,'new',1)`)
			var role string
			if err := db.QueryRow("SELECT role FROM tenant_members WHERE id=4").Scan(&role); err != nil || role != "viewer" {
				t.Fatal("default role is not employee", role, err)
			}
			exec(`INSERT INTO tenant_members(id,user_id,tenant_id,role) VALUES(5,'second-admin',1,'admin')`)
			var raw string
			if err := db.QueryRow("SELECT config FROM custom_agents WHERE id='custom'").Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if err := json.Unmarshal([]byte(raw), &config); err != nil {
				t.Fatal(err)
			}
			if config["system_prompt"] != "You are 环枢." || config["model_id"] != "WeKnora-model-id" || config["intent_prompts"].(map[string]any)["rewrite"] != "Ask 环枢" {
				t.Fatalf("brand migration changed wrong fields: %s", raw)
			}
		})
	}
}
