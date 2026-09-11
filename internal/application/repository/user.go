package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrUserNotFound                  = errors.New("user not found")
	ErrUserAlreadyExists             = errors.New("user already exists")
	ErrTokenNotFound                 = errors.New("token not found")
	ErrCannotRevokeSelf              = errors.New("cannot revoke your own system admin privileges")
	ErrLastSystemAdmin               = errors.New("cannot revoke the last remaining system administrator")
	ErrUserNotSystemAdmin            = errors.New("user is not a system administrator")
	ErrSystemAdminEnterpriseIdentity = errors.New("system administrator must be a tenantless platform identity")
)

// userRepository implements user repository interface
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *gorm.DB) interfaces.UserRepository {
	return &userRepository{db: db}
}

// CreateUser creates a user
func (r *userRepository) CreateUser(ctx context.Context, user *types.User) error {
	if user == nil {
		return errors.New("user is required")
	}
	if user.IsSystemAdmin {
		if user.TenantID != 0 || user.CanAccessAllTenants || isSpecialEnterpriseUserID(user.ID) {
			return ErrSystemAdminEnterpriseIdentity
		}
	}
	// users.tenant_id is nullable in both PostgreSQL and SQLite. GORM would
	// otherwise serialise the uint64 zero value as 0, which violates the
	// PostgreSQL FK and loses the distinction between "not provisioned yet"
	// and a real tenant. Omitting the column stores SQL NULL; reads hydrate it
	// back as zero, the domain sentinel used by tenantless auth flows.
	if user.TenantID == 0 {
		return r.db.WithContext(ctx).Omit("tenant_id").Create(user).Error
	}
	return r.db.WithContext(ctx).Create(user).Error
}

// GetUserByID gets a user by ID
func (r *userRepository) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	var user types.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUsersByIDs batch-fetches users by id with a single SELECT … WHERE id IN (…)
// and projects the result into a map keyed by user id. Returns an empty
// map for an empty input slice. Missing ids are silently absent from
// the result (consistent with the interface contract used by tenant
// member hydration).
func (r *userRepository) GetUsersByIDs(ctx context.Context, ids []string) (map[string]*types.User, error) {
	out := make(map[string]*types.User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var users []*types.User
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.ID] = u
	}
	return out, nil
}

// GetUserByEmail gets a user by email
func (r *userRepository) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	var user types.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername gets a user by username
func (r *userRepository) GetUserByUsername(ctx context.Context, username string) (*types.User, error) {
	var user types.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByTenantID gets the first user (owner) of a tenant
func (r *userRepository) GetUserByTenantID(ctx context.Context, tenantID uint64) (*types.User, error) {
	var user types.User
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at ASC").First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// UpdateUser updates a user
func (r *userRepository) UpdateUser(ctx context.Context, user *types.User) error {
	if user == nil {
		return errors.New("user is required")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current types.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", user.ID).Take(&current).Error; err != nil {
			return err
		}
		// Identity authority and enterprise binding have dedicated transactional
		// commands. A stale profile/preferences object must never grant, revoke,
		// or move those fields through this general update path.
		user.IsSystemAdmin = current.IsSystemAdmin
		user.CanAccessAllTenants = current.CanAccessAllTenants
		user.IsActive = current.IsActive
		user.PasswordHash = current.PasswordHash
		if current.IsSystemAdmin {
			// Platform identities are always tenantless. This also normalizes a
			// legacy mixed row when it passes through the general profile path.
			user.TenantID = 0
			user.CanAccessAllTenants = false
		} else if current.TenantID != 0 {
			user.TenantID = current.TenantID
		}
		if user.TenantID == 0 {
			// Preserve Save's all-fields behaviour while keeping the nullable
			// tenant column out of the struct write, then explicitly store NULL.
			// Writing uint64(0) would violate the PostgreSQL tenant FK.
			if err := tx.Omit("tenant_id").Save(user).Error; err != nil {
				return err
			}
			return tx.Model(&types.User{}).
				Where("id = ?", user.ID).
				UpdateColumn("tenant_id", nil).Error
		}
		return tx.Save(user).Error
	})
}

func (r *userRepository) UpdateCredential(
	ctx context.Context,
	userID, passwordHash string,
	preferences *types.UserPreferences,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current types.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ?", userID).Take(&current).Error; err != nil {
			return err
		}
		updates := map[string]any{"password_hash": passwordHash, "updated_at": time.Now()}
		if preferences != nil {
			updates["preferences"] = *preferences
		}
		res := tx.Model(&types.User{}).Where("id = ?", userID).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrUserNotFound
		}
		return nil
	})
}

func (r *userRepository) PromoteSystemAdmin(ctx context.Context, userID string) (*types.User, error) {
	var promoted types.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).Take(&promoted).Error; err != nil {
			return err
		}
		if promoted.IsSystemAdmin {
			if promoted.TenantID != 0 || promoted.CanAccessAllTenants || isSpecialEnterpriseUserID(promoted.ID) {
				return ErrSystemAdminEnterpriseIdentity
			}
			return nil
		}
		if !promoted.IsActive || promoted.TenantID != 0 || promoted.CanAccessAllTenants || isSpecialEnterpriseUserID(promoted.ID) {
			return ErrSystemAdminEnterpriseIdentity
		}
		var memberships int64
		if err := tx.Model(&types.TenantMember{}).Where("user_id = ?", promoted.ID).Count(&memberships).Error; err != nil {
			return err
		}
		if memberships != 0 {
			return ErrSystemAdminEnterpriseIdentity
		}
		res := tx.Model(&types.User{}).Where("id = ? AND is_system_admin = ?", promoted.ID, false).
			Updates(map[string]any{
				"is_system_admin":        true,
				"tenant_id":              nil,
				"can_access_all_tenants": false,
				"updated_at":             time.Now(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrSystemAdminEnterpriseIdentity
		}
		promoted.IsSystemAdmin = true
		promoted.TenantID = 0
		promoted.CanAccessAllTenants = false
		return nil
	})
	return &promoted, err
}

// DeleteUser deletes a user
func (r *userRepository) DeleteUser(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.User{}).Error
}

func (r *userRepository) DeleteTenantlessUser(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Unscoped().
		Where("id = ? AND (tenant_id IS NULL OR tenant_id = 0)", id).
		Delete(&types.User{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListUsers lists users with pagination
func (r *userRepository) ListUsers(ctx context.Context, offset, limit int) ([]*types.User, error) {
	var users []*types.User
	query := r.db.WithContext(ctx).Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// ListSystemAdmins lists users where is_system_admin = true.
//
// Walks idx_users_is_system_admin (created in migration 000052), so the
// query stays cheap even on a large users table — only the small subset
// of system admins is scanned. Returns total count alongside the page so
// the management UI can render pagination without a second roundtrip.
//
// Ordered by created_at DESC for stable, newest-first listing; ties are
// further broken by id to keep paging deterministic across boundaries.
// limit <= 0 means "no limit" (matches ListUsers semantics); callers in
// production pass a sane page size.
func (r *userRepository) ListSystemAdmins(ctx context.Context, offset, limit int) ([]*types.User, int64, error) {
	var users []*types.User
	var total int64

	base := r.db.WithContext(ctx).Model(&types.User{}).Where("is_system_admin = ?", true)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := base.Order("created_at DESC, id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	if err := query.Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// RevokeSystemAdmin revokes system-admin privileges inside a transaction.
// It locks the current admin rows before counting so concurrent revokes
// cannot both observe "two admins" and leave the platform with zero.
//
// Return contract:
//   - (user, nil): revoke actually happened; user.IsSystemAdmin == false
//   - (user, ErrUserNotSystemAdmin): target was already not an admin;
//     no row was written. Caller should treat as idempotent success but
//     MUST distinguish it from a real revoke for audit purposes — the
//     surfaced `user` is the unchanged DB row.
//   - (nil, ErrCannotRevokeSelf | ErrLastSystemAdmin | ErrUserNotFound | …):
//     hard rejection; no row written.
func (r *userRepository) RevokeSystemAdmin(ctx context.Context, userID, actorID string) (*types.User, error) {
	if userID == actorID {
		return nil, ErrCannotRevokeSelf
	}

	var revoked *types.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locking := func(db *gorm.DB) *gorm.DB {
			switch tx.Dialector.Name() {
			case "postgres", "mysql":
				return db.Clauses(clause.Locking{Strength: "UPDATE"})
			default:
				return db
			}
		}
		var user types.User
		if err := locking(tx).
			Where("id = ?", userID).
			First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		if !user.IsSystemAdmin {
			revoked = &user
			return ErrUserNotSystemAdmin
		}

		var admins []types.User
		if err := locking(tx).
			Where("is_system_admin = ?", true).
			Find(&admins).Error; err != nil {
			return err
		}
		if len(admins) <= 1 {
			return ErrLastSystemAdmin
		}

		now := time.Now()
		res := tx.Model(&types.User{}).
			Where("id = ? AND is_system_admin = ? AND (tenant_id IS NULL OR tenant_id = 0)", user.ID, true).
			Updates(map[string]any{
				"is_system_admin":        false,
				"can_access_all_tenants": false,
				"updated_at":             now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrSystemAdminEnterpriseIdentity
		}
		user.IsSystemAdmin = false
		user.CanAccessAllTenants = false
		user.UpdatedAt = now
		revoked = &user
		return nil
	})
	// Propagate ErrUserNotSystemAdmin up to the handler alongside the
	// (unchanged) user row. The handler treats it as idempotent success
	// but emits an audit row with changed=false so a probing pattern
	// ("revoke every random user id we know") still leaves a trail.
	if errors.Is(err, ErrUserNotSystemAdmin) {
		return revoked, err
	}
	if err != nil {
		return nil, err
	}
	return revoked, nil
}

// SearchUsers searches users by username or email
func (r *userRepository) SearchUsers(ctx context.Context, query string, limit int) ([]*types.User, error) {
	var users []*types.User
	searchPattern := "%" + query + "%"

	dbQuery := r.db.WithContext(ctx).
		Where("username ILIKE ? OR email ILIKE ?", searchPattern, searchPattern).
		Where("is_active = ?", true).
		Order("username ASC")

	if limit > 0 {
		dbQuery = dbQuery.Limit(limit)
	} else {
		dbQuery = dbQuery.Limit(20) // default limit
	}

	if err := dbQuery.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// authTokenRepository implements auth token repository interface
type authTokenRepository struct {
	db *gorm.DB
}

// NewAuthTokenRepository creates a new auth token repository
func NewAuthTokenRepository(db *gorm.DB) interfaces.AuthTokenRepository {
	return &authTokenRepository{db: db}
}

// CreateToken creates an auth token
func (r *authTokenRepository) CreateToken(ctx context.Context, token *types.AuthToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// GetTokenByValue gets a token by its value
func (r *authTokenRepository) GetTokenByValue(ctx context.Context, tokenValue string) (*types.AuthToken, error) {
	var token types.AuthToken
	if err := r.db.WithContext(ctx).Where("token = ?", tokenValue).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return &token, nil
}

// GetTokensByUserID gets all tokens for a user
func (r *authTokenRepository) GetTokensByUserID(ctx context.Context, userID string) ([]*types.AuthToken, error) {
	var tokens []*types.AuthToken
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

// UpdateToken updates a token
func (r *authTokenRepository) UpdateToken(ctx context.Context, token *types.AuthToken) error {
	return r.db.WithContext(ctx).Save(token).Error
}

// DeleteToken deletes a token
func (r *authTokenRepository) DeleteToken(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.AuthToken{}).Error
}

// DeleteExpiredTokens deletes all expired tokens
func (r *authTokenRepository) DeleteExpiredTokens(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("expires_at < NOW()").Delete(&types.AuthToken{}).Error
}

// RevokeTokensByUserID revokes all tokens for a user
func (r *authTokenRepository) RevokeTokensByUserID(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Model(&types.AuthToken{}).Where("user_id = ?", userID).Update("is_revoked", true).Error
}
