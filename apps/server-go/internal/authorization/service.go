package authorization

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed model.conf
var modelConfig string

// Domain converts an organization id to the Casbin RBAC-with-Domains value.
func Domain(orgID int64) string { return fmt.Sprintf("org::%d", orgID) }

type Subject struct {
	UserID             int64
	OrgID              int64
	PlatformSuperAdmin bool
}

type Permission struct {
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Resource   string `json:"resource"`
	Action     string `json:"action"`
	Type       string `json:"type"`
	ParentCode string `json:"parentCode,omitempty"`
	RoutePath  string `json:"routePath,omitempty"`
	SortOrder  int    `json:"sortOrder"`
	Status     string `json:"status"`
}

type Authorizer struct {
	db        *sql.DB
	mu        sync.RWMutex
	cache     map[string]cacheEntry
	cacheTTL  time.Duration
	enforcers *EnforcerManager
}

type cacheEntry struct {
	allowed bool
	expires time.Time
}

func New(db *sql.DB) *Authorizer {
	return &Authorizer{db: db, cache: make(map[string]cacheEntry), cacheTTL: 30 * time.Second, enforcers: NewEnforcerManager(db)}
}

func (a *Authorizer) InvalidateUser(userID int64) {
	a.mu.Lock()
	for k := range a.cache {
		if strings.Contains(k, fmt.Sprintf(":u:%d:", userID)) || strings.HasPrefix(k, fmt.Sprintf("u:%d:", userID)) {
			delete(a.cache, k)
		}
	}
	a.mu.Unlock()
	// A Casbin Enforcer contains role links for the whole tenant. User role
	// changes therefore invalidate the loaded tenant snapshots as well.
	a.enforcers.InvalidateAll()
}

func (a *Authorizer) InvalidateOrg(orgID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prefix := fmt.Sprintf("o:%d:", orgID)
	for k := range a.cache {
		if strings.HasPrefix(k, prefix) {
			delete(a.cache, k)
		}
	}
	a.enforcers.InvalidateOrg(orgID)
}

func (a *Authorizer) Check(ctx context.Context, subject Subject, permissionCode string) (bool, error) {
	permissionCode = strings.TrimSpace(permissionCode)
	if permissionCode == "" {
		return true, nil
	}
	if subject.UserID <= 0 || subject.OrgID <= 0 {
		return false, nil
	}
	if subject.PlatformSuperAdmin {
		return true, nil
	}
	key := fmt.Sprintf("o:%d:u:%d:%s", subject.OrgID, subject.UserID, permissionCode)
	a.mu.RLock()
	entry, ok := a.cache[key]
	a.mu.RUnlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.allowed, nil
	}
	allowed, err := a.enforcers.Enforce(ctx, subject, permissionCode)
	if err != nil {
		return false, err
	}
	a.mu.Lock()
	a.cache[key] = cacheEntry{allowed: allowed, expires: time.Now().Add(a.cacheTTL)}
	a.mu.Unlock()
	return allowed, nil
}

func (a *Authorizer) EffectivePermissions(ctx context.Context, subject Subject) ([]Permission, error) {
	if subject.UserID <= 0 || subject.OrgID <= 0 {
		return []Permission{}, nil
	}
	if subject.PlatformSuperAdmin {
		return a.allPermissions(ctx)
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT DISTINCT p.id, p.permission_code, p.resource, p.action, p.permission_type,
		       COALESCE(p.parent_code, ''), COALESCE(p.route_path, ''), p.sort_order, p.status
		FROM admin_user_role ur
		JOIN admin_role r ON r.org_id = ur.org_id AND r.id = ur.role_id AND r.status = 'ACTIVE'
		JOIN admin_role_permission rp ON rp.org_id = r.org_id AND rp.role_id = r.id
		JOIN admin_permission p ON p.id = rp.permission_id AND p.status = 'ACTIVE'
		JOIN admin_user u ON u.org_id = ur.org_id AND u.id = ur.user_id AND u.status = 'ACTIVE'
		WHERE ur.org_id = $1 AND ur.user_id = $2
		ORDER BY p.sort_order, p.id`, subject.OrgID, subject.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPermissions(rows)
}

func (a *Authorizer) allPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, permission_code, resource, action, permission_type, COALESCE(parent_code, ''), COALESCE(route_path, ''), sort_order, status FROM admin_permission WHERE status = 'ACTIVE' ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPermissions(rows)
}

func scanPermissions(rows *sql.Rows) ([]Permission, error) {
	result := make([]Permission, 0)
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Code, &p.Resource, &p.Action, &p.Type, &p.ParentCode, &p.RoutePath, &p.SortOrder, &p.Status); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// CheckWithDefaultDeny is used by the HTTP boundary. Database failures never grant access.
func (a *Authorizer) CheckWithDefaultDeny(ctx context.Context, subject Subject, code string) bool {
	ok, err := a.Check(ctx, subject, code)
	return err == nil && ok
}

var ErrForbidden = errors.New("forbidden")

func Require(a *Authorizer, subject Subject, code string, r *http.Request) error {
	ok, err := a.Check(r.Context(), subject, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
