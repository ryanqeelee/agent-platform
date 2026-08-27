package service

// Knowledge governance is implemented by the transactional GORM service in
// repository/knowledge_governance.go. Keeping the exported constructor there
// avoids a second repository interface and preserves one authoritative
// read/write point for grants, role state, and access checks.
