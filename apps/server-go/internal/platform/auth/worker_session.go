package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fixpro/server/internal/platform/httpx"
)

const WorkerSessionHeader = "Authorization"

var (
	ErrWorkerLoginFailed      = errors.New("worker login failed")
	ErrWorkerSessionInvalid   = errors.New("worker session invalid")
	ErrWorkerPasswordRequired = errors.New("worker password change required")
	ErrWorkerCurrentPassword  = errors.New("worker current password invalid")
	ErrWorkerPasswordMismatch = errors.New("worker password mismatch")
	ErrWorkerPasswordTooWeak  = errors.New("worker password too weak")
)

type WorkerSessionStore struct {
	db             *sql.DB
	ttl            time.Duration
	devTokenEnable bool
}

type WorkerLoginResult struct {
	Principal          Principal
	Token              string
	ExpiresAt          time.Time
	MustChangePassword bool
	WorkerNo           string
	Mobile             string
}

func NewWorkerSessionStore(db *sql.DB, devTokenEnable bool) *WorkerSessionStore {
	return &WorkerSessionStore{db: db, ttl: 12 * time.Hour, devTokenEnable: devTokenEnable}
}

func (s *WorkerSessionStore) Login(ctx context.Context, orgID int64, mobile, password, userAgent, ip string) (WorkerLoginResult, error) {
	mobile = strings.TrimSpace(mobile)
	if orgID <= 0 {
		orgID = 1
	}
	var (
		id, passwordVersion                                      int64
		displayName, workerNo, encodedHash, status, storedMobile string
		mustChange                                               bool
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(worker_no,''), display_name, COALESCE(mobile,''), password_hash,
		       status, must_change_password, password_version
		FROM employee_account
		WHERE org_id=$1 AND role='WORKER' AND deleted_at IS NULL AND mobile=$2`, orgID, mobile).
		Scan(&id, &workerNo, &displayName, &storedMobile, &encodedHash, &status, &mustChange, &passwordVersion)
	if err != nil || status != "ACTIVE" || storedMobile == "" || !VerifyPassword(password, encodedHash) {
		return WorkerLoginResult{}, ErrWorkerLoginFailed
	}
	token, err := randomToken(32)
	if err != nil {
		return WorkerLoginResult{}, err
	}
	expires := time.Now().UTC().Add(s.ttl)
	if err = s.db.QueryRowContext(ctx, `
		INSERT INTO worker_session(org_id,worker_id,token_hash,password_version,expires_at,user_agent,ip_address)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, orgID, id, sha256Bytes(token), passwordVersion, expires, userAgent, ip).Scan(new(int64)); err != nil {
		return WorkerLoginResult{}, err
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE employee_account SET last_login_at=CURRENT_TIMESTAMP(3) WHERE org_id=$1 AND id=$2`, orgID, id); err != nil {
		return WorkerLoginResult{}, err
	}
	p := Principal{
		OrgID: orgID, SubjectID: id, Role: "WORKER", Name: displayName,
		SessionAuthenticated: true, MustChangePassword: mustChange,
	}
	return WorkerLoginResult{Principal: p, Token: token, ExpiresAt: expires, MustChangePassword: mustChange, WorkerNo: workerNo, Mobile: storedMobile}, nil
}

func (s *WorkerSessionStore) Authenticate(r *http.Request) (Principal, string, error) {
	header := strings.TrimSpace(r.Header.Get(WorkerSessionHeader))
	if !strings.HasPrefix(header, "Bearer ") {
		return Principal{}, "", ErrWorkerSessionInvalid
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return Principal{}, "", ErrWorkerSessionInvalid
	}
	if s.devTokenEnable && strings.HasPrefix(token, "local-worker-") {
		var p Principal
		var status string
		var displayName string
		var id int64
		if parsed, err := strconv.ParseInt(strings.TrimPrefix(token, "local-worker-"), 10, 64); err != nil || parsed <= 0 {
			return Principal{}, "", ErrWorkerSessionInvalid
		} else {
			id = parsed
		}
		if err := s.db.QueryRowContext(r.Context(), `SELECT display_name,status FROM employee_account WHERE org_id=1 AND id=$1 AND role='WORKER' AND deleted_at IS NULL`, id).Scan(&displayName, &status); err != nil || status != "ACTIVE" {
			return Principal{}, "", ErrWorkerSessionInvalid
		}
		p = Principal{OrgID: 1, SubjectID: id, Role: "WORKER", Name: displayName, SessionAuthenticated: true}
		return p, token, nil
	}

	var p Principal
	var expires time.Time
	var revokedAt sql.NullTime
	var passwordVersion, sessionVersion int
	var status string
	err := s.db.QueryRowContext(r.Context(), `
		SELECT ws.org_id, ws.worker_id, ws.expires_at, ws.revoked_at, ws.password_version,
		       e.display_name, e.status, e.must_change_password, e.password_version
		FROM worker_session ws
		JOIN employee_account e ON e.org_id=ws.org_id AND e.id=ws.worker_id AND e.role='WORKER' AND e.deleted_at IS NULL
		WHERE ws.token_hash=$1`, sha256Bytes(token)).Scan(&p.OrgID, &p.SubjectID, &expires, &revokedAt, &sessionVersion, &p.Name, &status, &p.MustChangePassword, &passwordVersion)
	if err != nil || revokedAt.Valid || expires.Before(time.Now().UTC()) || status != "ACTIVE" || sessionVersion != passwordVersion {
		return Principal{}, "", ErrWorkerSessionInvalid
	}
	p.Role = "WORKER"
	p.SessionAuthenticated = true
	_, _ = s.db.ExecContext(r.Context(), `UPDATE worker_session SET last_seen_at=CURRENT_TIMESTAMP(3) WHERE token_hash=$1 AND revoked_at IS NULL`, sha256Bytes(token))
	return p, token, nil
}

func (s *WorkerSessionStore) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE worker_session SET revoked_at=CURRENT_TIMESTAMP(3) WHERE token_hash=$1 AND revoked_at IS NULL`, sha256Bytes(token))
	return err
}

func (s *WorkerSessionStore) RevokeWorkerSessions(ctx context.Context, orgID, workerID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE worker_session SET revoked_at=CURRENT_TIMESTAMP(3) WHERE org_id=$1 AND worker_id=$2 AND revoked_at IS NULL`, orgID, workerID)
	return err
}

func (s *WorkerSessionStore) ChangePassword(ctx context.Context, p Principal, token, currentPassword, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" || len([]rune(newPassword)) < 12 || !hasLetterAndDigit(newPassword) {
		return ErrWorkerPasswordTooWeak
	}
	if currentPassword == newPassword {
		return ErrWorkerPasswordTooWeak
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var encoded string
	var mobile string
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT password_hash,COALESCE(mobile,''),password_version FROM employee_account WHERE org_id=$1 AND id=$2 AND role='WORKER' AND status='ACTIVE' FOR UPDATE`, p.OrgID, p.SubjectID).Scan(&encoded, &mobile, &version); err != nil || !VerifyPassword(currentPassword, encoded) {
		return ErrWorkerCurrentPassword
	}
	if newPassword == "w"+mobile {
		return ErrWorkerPasswordTooWeak
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE employee_account SET password_hash=$1,must_change_password=FALSE,password_version=password_version+1,last_password_changed_at=CURRENT_TIMESTAMP(3),version=version+1 WHERE org_id=$2 AND id=$3`, hash, p.OrgID, p.SubjectID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE worker_session SET revoked_at=CURRENT_TIMESTAMP(3) WHERE org_id=$1 AND worker_id=$2 AND revoked_at IS NULL`, p.OrgID, p.SubjectID); err != nil {
		return err
	}
	return tx.Commit()
}

func hasLetterAndDigit(value string) bool {
	hasLetter, hasDigit := false, false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	return hasLetter && hasDigit
}

func WorkerSession(store *WorkerSessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _, err := store.Authenticate(r)
		if err != nil {
			httpx.Failure(w, r, httpx.E("WORKER_SESSION_INVALID", "需要师傅登录", http.StatusUnauthorized))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}
