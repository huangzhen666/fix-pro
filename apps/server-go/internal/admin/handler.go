package admin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fixpro/server/internal/authorization"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
)

type Handler struct {
	db         *sql.DB
	sessions   *auth.SessionStore
	authorizer *authorization.Authorizer
}

func NewHandler(db *sql.DB, sessions *auth.SessionStore, authorizer *authorization.Authorizer) *Handler {
	return &Handler{db: db, sessions: sessions, authorizer: authorizer}
}

type loginRequest struct {
	OrgID    int64  `json:"orgId"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in loginRequest
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	result, err := h.sessions.Login(r.Context(), in.OrgID, in.Username, in.Password, r.UserAgent(), clientIP(r))
	if errors.Is(err, auth.ErrInvalidCredentials) {
		httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "用户名或密码错误", 401))
		return
	}
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	h.sessions.SetLoginCookies(w, result, result.SessionToken)
	h.writeAudit(r.Context(), result.Principal, "auth.login", "admin_user", strconv.FormatInt(result.Principal.AdminUserID, 10), nil)
	httpx.Success(w, r, map[string]any{"user": result.Principal, "mustChangePassword": result.MustChangePassword})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookieName); err == nil {
		_ = h.sessions.Logout(r.Context(), c.Value)
	}
	h.sessions.ClearCookies(w)
	httpx.Success(w, r, map[string]bool{"loggedOut": true})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.From(r.Context())
	if !ok || !p.SessionAuthenticated {
		httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "需要管理员登录", 401))
		return
	}
	permissions, err := h.authorizer.EffectivePermissions(r.Context(), authorization.Subject{UserID: p.AdminUserID, OrgID: p.OrgID, PlatformSuperAdmin: p.PlatformSuperAdmin})
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]any{"user": p, "permissions": permissions})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
		ConfirmPassword string `json:"confirmPassword"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if in.NewPassword != in.ConfirmPassword {
		httpx.Failure(w, r, httpx.E("VALIDATION_ERROR", "两次输入的新密码不一致", 400))
		return
	}
	var encoded string
	if err := h.db.QueryRowContext(r.Context(), `SELECT password_hash FROM admin_user WHERE org_id=$1 AND id=$2 AND status='ACTIVE'`, p.OrgID, p.AdminUserID).Scan(&encoded); err != nil || !auth.VerifyPassword(in.CurrentPassword, encoded) {
		httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "当前密码错误", 401))
		return
	}
	hash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		httpx.Failure(w, r, httpx.E("VALIDATION_ERROR", err.Error(), 400))
		return
	}
	_, err = h.db.ExecContext(r.Context(), `UPDATE admin_user SET password_hash=$1, must_change_password=FALSE, version=version+1 WHERE org_id=$2 AND id=$3 AND version=(SELECT version FROM admin_user WHERE org_id=$2 AND id=$3)`, hash, p.OrgID, p.AdminUserID)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	h.writeAudit(r.Context(), p, "admin_user.change_password", "admin_user", strconv.FormatInt(p.AdminUserID, 10), nil)
	httpx.Success(w, r, map[string]bool{"changed": true})
}

func (h *Handler) Permissions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, permission_code, resource, action, permission_type, COALESCE(parent_code,''), COALESCE(route_path,''), sort_order, status FROM admin_permission WHERE status='ACTIVE' ORDER BY sort_order,id`)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer rows.Close()
	result := make([]authorization.Permission, 0)
	for rows.Next() {
		var p authorization.Permission
		if err := rows.Scan(&p.ID, &p.Code, &p.Resource, &p.Action, &p.Type, &p.ParentCode, &p.RoutePath, &p.SortOrder, &p.Status); err != nil {
			httpx.Failure(w, r, err)
			return
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, result)
}

type Role struct {
	ID              int64     `json:"id"`
	OrgID           int64     `json:"orgId"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	IsBuiltin       bool      `json:"isBuiltin"`
	Version         int       `json:"version"`
	PermissionCodes []string  `json:"permissionCodes,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (h *Handler) Roles(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, org_id, role_code, name, COALESCE(description,''), status, is_builtin, version, created_at, updated_at FROM admin_role WHERE org_id=$1 ORDER BY created_at DESC,id DESC`, p.OrgID)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]Role, 0)
	for rows.Next() {
		var x Role
		if err := rows.Scan(&x.ID, &x.OrgID, &x.Code, &x.Name, &x.Description, &x.Status, &x.IsBuiltin, &x.Version, &x.CreatedAt, &x.UpdatedAt); err != nil {
			httpx.Failure(w, r, err)
			return
		}
		out = append(out, x)
	}
	httpx.Success(w, r, out)
}

func (h *Handler) RoleDetail(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	x, err := h.getRole(r.Context(), p.OrgID, id)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, x)
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var in struct {
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		PermissionCodes []string `json:"permissionCodes"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		httpx.Failure(w, r, httpx.E("VALIDATION_ERROR", "角色名称不能为空", 400))
		return
	}
	code := fmt.Sprintf("role_%d", time.Now().UnixNano())
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(r.Context(), `INSERT INTO admin_role(org_id,role_code,name,description) VALUES($1,$2,$3,$4) RETURNING id`, p.OrgID, code, strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)).Scan(&id)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.replaceRolePermissions(r.Context(), tx, p.OrgID, id, in.PermissionCodes); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	h.authorizer.InvalidateOrg(p.OrgID)
	h.writeAudit(r.Context(), p, "admin_role.create", "admin_role", strconv.FormatInt(id, 10), in)
	x, _ := h.getRole(r.Context(), p.OrgID, id)
	httpx.Success(w, r, x)
}

func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var in struct {
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		Status          string   `json:"status"`
		Version         int      `json:"version"`
		PermissionCodes []string `json:"permissionCodes"`
	}
	if err = httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer tx.Rollback()
	var n int64
	err = tx.QueryRowContext(r.Context(), `UPDATE admin_role SET name=COALESCE(NULLIF($1,''),name),description=$2,status=COALESCE(NULLIF($3,''),status),version=version+1 WHERE org_id=$4 AND id=$5 AND is_builtin=FALSE AND version=$6 RETURNING id`, strings.TrimSpace(in.Name), in.Description, in.Status, p.OrgID, id, in.Version).Scan(&n)
	if err != nil {
		httpx.Failure(w, r, httpx.E("CONFLICT", "角色已被修改或不可修改", 409))
		return
	}
	if err = h.replaceRolePermissions(r.Context(), tx, p.OrgID, id, in.PermissionCodes); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	h.authorizer.InvalidateOrg(p.OrgID)
	h.writeAudit(r.Context(), p, "admin_role.update", "admin_role", strconv.FormatInt(id, 10), in)
	x, _ := h.getRole(r.Context(), p.OrgID, id)
	httpx.Success(w, r, x)
}

func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var builtin bool
	if err = h.db.QueryRowContext(r.Context(), `SELECT is_builtin FROM admin_role WHERE org_id=$1 AND id=$2`, p.OrgID, id).Scan(&builtin); err != nil || builtin {
		httpx.Failure(w, r, httpx.E("CONFLICT", "内置角色不能删除", 409))
		return
	}
	var used bool
	_ = h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM admin_user_role WHERE org_id=$1 AND role_id=$2)`, p.OrgID, id).Scan(&used)
	if used {
		httpx.Failure(w, r, httpx.E("CONFLICT", "角色仍被用户使用，不能删除", 409))
		return
	}
	if _, err = h.db.ExecContext(r.Context(), `DELETE FROM admin_role WHERE org_id=$1 AND id=$2`, p.OrgID, id); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	h.authorizer.InvalidateOrg(p.OrgID)
	h.writeAudit(r.Context(), p, "admin_role.delete", "admin_role", strconv.FormatInt(id, 10), nil)
	httpx.Success(w, r, map[string]bool{"deleted": true})
}

func (h *Handler) replaceRolePermissions(ctx context.Context, tx *sql.Tx, orgID, roleID int64, codes []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_role_permission WHERE org_id=$1 AND role_id=$2`, orgID, roleID); err != nil {
		return err
	}
	for _, code := range codes {
		var pid int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM admin_permission WHERE permission_code=$1 AND status='ACTIVE'`, strings.TrimSpace(code)).Scan(&pid); err != nil {
			return httpx.E("VALIDATION_ERROR", "存在无效权限编码", 400)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO admin_role_permission(org_id,role_id,permission_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, orgID, roleID, pid); err != nil {
			return err
		}
	}
	return nil
}
func (h *Handler) getRole(ctx context.Context, orgID, id int64) (Role, error) {
	var x Role
	err := h.db.QueryRowContext(ctx, `SELECT id,org_id,role_code,name,COALESCE(description,''),status,is_builtin,version,created_at,updated_at FROM admin_role WHERE org_id=$1 AND id=$2`, orgID, id).Scan(&x.ID, &x.OrgID, &x.Code, &x.Name, &x.Description, &x.Status, &x.IsBuiltin, &x.Version, &x.CreatedAt, &x.UpdatedAt)
	if err != nil {
		return x, err
	}
	rows, err := h.db.QueryContext(ctx, `SELECT p.permission_code FROM admin_role_permission rp JOIN admin_permission p ON p.id=rp.permission_id WHERE rp.org_id=$1 AND rp.role_id=$2 ORDER BY p.sort_order,p.id`, orgID, id)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return x, err
		}
		x.PermissionCodes = append(x.PermissionCodes, c)
	}
	return x, rows.Err()
}

type User struct {
	ID                 int64     `json:"id"`
	OrgID              int64     `json:"orgId"`
	Username           string    `json:"username"`
	DisplayName        string    `json:"displayName"`
	Status             string    `json:"status"`
	MustChangePassword bool      `json:"mustChangePassword"`
	Version            int       `json:"version"`
	RoleIDs            []int64   `json:"roleIds,omitempty"`
	RoleNames          []string  `json:"roleNames,omitempty"`
	RoleCodes          []string  `json:"roleCodes,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT u.id,u.org_id,u.username,u.display_name,u.status,u.must_change_password,u.version,u.created_at,u.updated_at,
		       role_summary.role_names,role_summary.role_codes
		FROM admin_user u
		LEFT JOIN LATERAL (
			SELECT COALESCE(string_agg(DISTINCT ar.name, '、' ORDER BY ar.name), '') AS role_names,
			       COALESCE(string_agg(DISTINCT ar.role_code, ',' ORDER BY ar.role_code), '') AS role_codes
			FROM admin_user_role aur
			JOIN admin_role ar ON ar.org_id=aur.org_id AND ar.id=aur.role_id
			WHERE aur.org_id=u.org_id AND aur.user_id=u.id AND ar.status='ACTIVE'
		) role_summary ON TRUE
		WHERE u.org_id=$1 AND ($2='' OR u.username ILIKE '%'||$2||'%' OR u.display_name ILIKE '%'||$2||'%')
		ORDER BY u.created_at DESC,u.id DESC`, p.OrgID, r.URL.Query().Get("keyword"))
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]User, 0)
	for rows.Next() {
		var x User
		var roleNames, roleCodes string
		if err := rows.Scan(&x.ID, &x.OrgID, &x.Username, &x.DisplayName, &x.Status, &x.MustChangePassword, &x.Version, &x.CreatedAt, &x.UpdatedAt, &roleNames, &roleCodes); err != nil {
			httpx.Failure(w, r, err)
			return
		}
		if roleNames != "" {
			x.RoleNames = strings.Split(roleNames, "、")
		}
		if roleCodes != "" {
			x.RoleCodes = strings.Split(roleCodes, ",")
		}
		out = append(out, x)
	}
	httpx.Success(w, r, out)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var in struct {
		Username    string  `json:"username"`
		DisplayName string  `json:"displayName"`
		RoleIDs     []int64 `json:"roleIds"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if strings.TrimSpace(in.Username) == "" || strings.TrimSpace(in.DisplayName) == "" {
		httpx.Failure(w, r, httpx.E("VALIDATION_ERROR", "用户名和显示名不能为空", 400))
		return
	}
	temp, err := temporaryPassword()
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	hash, err := auth.HashPassword(temp)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer tx.Rollback()
	var id int64
	if err = tx.QueryRowContext(r.Context(), `INSERT INTO admin_user(org_id,username,display_name,password_hash,must_change_password) VALUES($1,$2,$3,$4,TRUE) RETURNING id`, p.OrgID, strings.TrimSpace(in.Username), strings.TrimSpace(in.DisplayName), hash).Scan(&id); err != nil {
		httpx.Failure(w, r, httpx.E("CONFLICT", "用户名已存在", 409))
		return
	}
	for _, roleID := range in.RoleIDs {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO admin_user_role(org_id,user_id,role_id) SELECT $1,$2,id FROM admin_role WHERE org_id=$1 AND id=$3`, p.OrgID, id, roleID); err != nil {
			httpx.Failure(w, r, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	h.authorizer.InvalidateUser(id)
	h.writeAudit(r.Context(), p, "admin_user.create", "admin_user", strconv.FormatInt(id, 10), nil)
	httpx.Success(w, r, map[string]any{"id": id, "temporaryPassword": temp})
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var in struct {
		DisplayName string  `json:"displayName"`
		Status      string  `json:"status"`
		Version     int     `json:"version"`
		RoleIDs     []int64 `json:"roleIds"`
	}
	if err = httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer tx.Rollback()
	var n int64
	err = tx.QueryRowContext(r.Context(), `UPDATE admin_user SET display_name=COALESCE(NULLIF($1,''),display_name),status=COALESCE(NULLIF($2,''),status),version=version+1 WHERE org_id=$3 AND id=$4 AND version=$5 RETURNING id`, strings.TrimSpace(in.DisplayName), in.Status, p.OrgID, id, in.Version).Scan(&n)
	if err != nil {
		httpx.Failure(w, r, httpx.E("CONFLICT", "用户已被修改", 409))
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM admin_user_role WHERE org_id=$1 AND user_id=$2`, p.OrgID, id); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	for _, roleID := range in.RoleIDs {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO admin_user_role(org_id,user_id,role_id) SELECT $1,$2,id FROM admin_role WHERE org_id=$1 AND id=$3`, p.OrgID, id, roleID); err != nil {
			httpx.Failure(w, r, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	_ = h.sessions.RevokeUserSessions(r.Context(), p.OrgID, id)
	h.authorizer.InvalidateUser(id)
	h.writeAudit(r.Context(), p, "admin_user.update", "admin_user", strconv.FormatInt(id, 10), nil)
	httpx.Success(w, r, map[string]bool{"updated": true})
}

func (h *Handler) SetUserStatus(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var in struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if err = httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if in.Status != "ACTIVE" && in.Status != "DISABLED" && in.Status != "LOCKED" {
		httpx.Failure(w, r, httpx.E("VALIDATION_ERROR", "用户状态无效", 400))
		return
	}
	var isTenantAdmin, isPlatform bool
	_ = h.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM admin_user_role ur JOIN admin_role ar ON ar.org_id=ur.org_id AND ar.id=ur.role_id WHERE ur.org_id=$1 AND ur.user_id=$2 AND ar.role_code='tenant_admin'), EXISTS (SELECT 1 FROM admin_platform_super_admin WHERE user_id=$2)`, p.OrgID, id).Scan(&isTenantAdmin, &isPlatform)
	if in.Status != "ACTIVE" && (isPlatform || isTenantAdmin) {
		var activeAdmins int
		_ = h.db.QueryRowContext(r.Context(), `SELECT count(*) FROM admin_user u JOIN admin_user_role ur ON ur.org_id=u.org_id AND ur.user_id=u.id JOIN admin_role ar ON ar.org_id=ur.org_id AND ar.id=ur.role_id AND ar.role_code='tenant_admin' WHERE u.org_id=$1 AND u.status='ACTIVE'`, p.OrgID).Scan(&activeAdmins)
		if activeAdmins <= 1 {
			httpx.Failure(w, r, httpx.E("CONFLICT", "不能禁用租户最后一名管理员", 409))
			return
		}
	}
	var n int64
	err = h.db.QueryRowContext(r.Context(), `UPDATE admin_user SET status=$1, version=version+1 WHERE org_id=$2 AND id=$3 AND version=$4 RETURNING id`, in.Status, p.OrgID, id, in.Version).Scan(&n)
	if err != nil {
		httpx.Failure(w, r, httpx.E("CONFLICT", "用户已被修改", 409))
		return
	}
	if in.Status != "ACTIVE" {
		_ = h.sessions.RevokeUserSessions(r.Context(), p.OrgID, id)
	}
	h.authorizer.InvalidateUser(id)
	h.writeAudit(r.Context(), p, "admin_user.status", "admin_user", strconv.FormatInt(id, 10), in)
	httpx.Success(w, r, map[string]bool{"updated": true})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	temp, err := temporaryPassword()
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	hash, err := auth.HashPassword(temp)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if _, err = h.db.ExecContext(r.Context(), `UPDATE admin_user SET password_hash=$1,must_change_password=TRUE,version=version+1 WHERE org_id=$2 AND id=$3`, hash, p.OrgID, id); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	_ = h.sessions.RevokeUserSessions(r.Context(), p.OrgID, id)
	h.authorizer.InvalidateUser(id)
	h.writeAudit(r.Context(), p, "admin_user.reset_password", "admin_user", strconv.FormatInt(id, 10), nil)
	httpx.Success(w, r, map[string]any{"temporaryPassword": temp})
}

func (h *Handler) EffectivePermissions(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	x, err := h.authorizer.EffectivePermissions(r.Context(), authorization.Subject{UserID: id, OrgID: p.OrgID})
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, x)
}

func (h *Handler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	limit := 100
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, principal_id, action_code, resource_type, COALESCE(resource_id,''), request_id, created_at FROM audit_log WHERE org_id=$1 ORDER BY created_at DESC, id DESC LIMIT $2`, p.OrgID, limit)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID           int64     `json:"id"`
		PrincipalID  *int64    `json:"principalId,omitempty"`
		ActionCode   string    `json:"actionCode"`
		ResourceType string    `json:"resourceType"`
		ResourceID   string    `json:"resourceId"`
		RequestID    *string   `json:"requestId,omitempty"`
		CreatedAt    time.Time `json:"createdAt"`
	}
	out := make([]item, 0)
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.ID, &x.PrincipalID, &x.ActionCode, &x.ResourceType, &x.ResourceID, &x.RequestID, &x.CreatedAt); err != nil {
			httpx.Failure(w, r, err)
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}

func (h *Handler) writeAudit(ctx context.Context, p auth.Principal, action, resource, id string, detail any) {
	if p.OrgID <= 0 {
		return
	}
	_, _ = h.db.ExecContext(ctx, `INSERT INTO audit_log(org_id,principal_id,action_code,resource_type,resource_id,detail_json) VALUES($1,$2,$3,$4,$5,$6)`, p.OrgID, p.AdminUserID, action, resource, id, detail)
}
func clientIP(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		v = r.RemoteAddr
	}
	return v
}
func temporaryPassword() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "Fx!" + base64.RawURLEncoding.EncodeToString(b), nil
}
