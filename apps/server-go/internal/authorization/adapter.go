package authorization

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
)

// PostgresAdapter is a read-only Casbin adapter backed by the normalized RBAC
// tables. It deliberately does not create a duplicate casbin_rule table.
type PostgresAdapter struct {
	db    *sql.DB
	orgID int64
	ctx   context.Context
}

func NewPostgresAdapter(ctx context.Context, db *sql.DB, orgID int64) *PostgresAdapter {
	if ctx == nil {
		ctx = context.Background()
	}
	return &PostgresAdapter{db: db, orgID: orgID, ctx: ctx}
}

func (a *PostgresAdapter) LoadPolicy(m casbinmodel.Model) error {
	if a.orgID <= 0 {
		return errors.New("invalid authorization domain")
	}
	// p = role, domain, permission_code, *, allow
	rows, err := a.db.QueryContext(a.ctx, `
		SELECT r.role_code, r.org_id, p.permission_code
		FROM admin_role r
		JOIN admin_role_permission rp ON rp.org_id = r.org_id AND rp.role_id = r.id
		JOIN admin_permission p ON p.id = rp.permission_id AND p.status = 'ACTIVE'
		WHERE r.org_id = $1 AND r.status = 'ACTIVE'
		ORDER BY r.id, p.id`, a.orgID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var role string
		var orgID int64
		var permission string
		if err := rows.Scan(&role, &orgID, &permission); err != nil {
			rows.Close()
			return err
		}
		line := fmt.Sprintf("p, role:%s, %s, %s, *, allow", role, Domain(orgID), permission)
		if err := persist.LoadPolicyLine(line, m); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// g = user, role, domain. Only active users and active roles are loaded.
	rows, err = a.db.QueryContext(a.ctx, `
		SELECT u.id, r.role_code, u.org_id
		FROM admin_user u
		JOIN admin_user_role ur ON ur.org_id = u.org_id AND ur.user_id = u.id
		JOIN admin_role r ON r.org_id = ur.org_id AND r.id = ur.role_id
		WHERE u.org_id = $1 AND u.status = 'ACTIVE' AND r.status = 'ACTIVE'
		ORDER BY u.id, r.id`, a.orgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID, orgID int64
		var role string
		if err := rows.Scan(&userID, &role, &orgID); err != nil {
			return err
		}
		line := "g, user:" + strconv.FormatInt(userID, 10) + ", role:" + role + ", " + Domain(orgID)
		if err := persist.LoadPolicyLine(line, m); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (a *PostgresAdapter) SavePolicy(casbinmodel.Model) error {
	return errors.New("authorization policy is managed by PostgreSQL RBAC tables")
}
func (a *PostgresAdapter) AddPolicy(string, string, []string) error {
	return errors.New("authorization policy is managed by PostgreSQL RBAC tables")
}
func (a *PostgresAdapter) RemovePolicy(string, string, []string) error {
	return errors.New("authorization policy is managed by PostgreSQL RBAC tables")
}
func (a *PostgresAdapter) RemoveFilteredPolicy(string, string, int, ...string) error {
	return errors.New("authorization policy is managed by PostgreSQL RBAC tables")
}
