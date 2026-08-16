package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	SessionCookieName = "fixpro_admin_session"
	CSRFCookieName    = "fixpro_admin_csrf"
	CSRFHeaderName    = "X-CSRF-Token"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrSessionExpired = errors.New("session expired")
var ErrPasswordChangeRequired = errors.New("password change required")

type SessionStore struct {
	db           *sql.DB
	cookieSecure bool
	sessionTTL   time.Duration
}

type LoginResult struct {
	Principal          Principal
	SessionID          int64
	SessionToken       string
	CSRFToken          string
	MustChangePassword bool
}

func NewSessionStore(db *sql.DB, secureCookie bool) *SessionStore {
	return &SessionStore{db: db, cookieSecure: secureCookie, sessionTTL: 12 * time.Hour}
}

func (s *SessionStore) Login(ctx context.Context, orgID int64, username, password, userAgent, ip string) (LoginResult, error) {
	if orgID <= 0 {
		orgID = 1
	}
	var userID int64
	var displayName, encodedHash, status string
	var mustChange bool
	err := s.db.QueryRowContext(ctx, `SELECT id, display_name, password_hash, status, must_change_password FROM admin_user WHERE org_id = $1 AND username = $2`, orgID, strings.TrimSpace(username)).Scan(&userID, &displayName, &encodedHash, &status, &mustChange)
	if errors.Is(err, sql.ErrNoRows) || err != nil || status != "ACTIVE" || !VerifyPassword(password, encodedHash) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if NeedsPasswordRehash(encodedHash) {
		migrated, hashErr := HashPassword(password)
		if hashErr != nil {
			return LoginResult{}, hashErr
		}
		if _, hashErr = s.db.ExecContext(ctx, `UPDATE admin_user SET password_hash=$1, version=version+1 WHERE org_id=$2 AND id=$3`, migrated, orgID, userID); hashErr != nil {
			return LoginResult{}, hashErr
		}
	}
	token, err := randomToken(32)
	if err != nil {
		return LoginResult{}, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return LoginResult{}, err
	}
	expires := time.Now().UTC().Add(s.sessionTTL)
	result, err := s.db.ExecContext(ctx, `INSERT INTO admin_user_session (org_id, user_id, token_hash, csrf_token_hash, user_agent, ip_address, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, orgID, userID, sha256Bytes(token), sha256Bytes(csrf), userAgent, ip, expires)
	if err != nil {
		return LoginResult{}, err
	}
	sessionID, err := result.LastInsertId()
	if err != nil {
		_ = s.db.QueryRowContext(ctx, `SELECT id FROM admin_user_session WHERE token_hash = $1`, sha256Bytes(token)).Scan(&sessionID)
	}
	return LoginResult{Principal: Principal{OrgID: orgID, SubjectID: userID, AdminUserID: userID, Role: "ADMIN", Name: displayName, SessionID: sessionID, SessionAuthenticated: true, MustChangePassword: mustChange}, SessionID: sessionID, SessionToken: token, CSRFToken: csrf, MustChangePassword: mustChange}, nil
}

func (s *SessionStore) Authenticate(r *http.Request) (Principal, string, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return Principal{}, "", ErrSessionExpired
	}
	tokenHash := sha256Bytes(cookie.Value)
	var p Principal
	var expires time.Time
	var revokedAt sql.NullTime
	var csrfHash []byte
	var status string
	var platform bool
	var mustChange bool
	err = s.db.QueryRowContext(r.Context(), `
		SELECT s.id, s.org_id, s.user_id, s.csrf_token_hash, s.expires_at, s.revoked_at,
		       u.display_name, u.status, u.must_change_password,
		       EXISTS (SELECT 1 FROM admin_platform_super_admin psa WHERE psa.user_id = s.user_id)
		FROM admin_user_session s JOIN admin_user u ON u.id = s.user_id AND u.org_id = s.org_id
		WHERE s.token_hash = $1`, tokenHash).Scan(&p.SessionID, &p.OrgID, &p.SubjectID, &csrfHash, &expires, &revokedAt, &p.Name, &status, &mustChange, &platform)
	if err != nil || revokedAt.Valid || expires.Before(time.Now().UTC()) || status != "ACTIVE" {
		return Principal{}, "", ErrSessionExpired
	}
	p.AdminUserID = p.SubjectID
	p.Role = "ADMIN"
	p.SessionAuthenticated = true
	p.PlatformSuperAdmin = platform
	p.MustChangePassword = mustChange
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		csrf := strings.TrimSpace(r.Header.Get(CSRFHeaderName))
		if csrf == "" || !hmac.Equal(csrfHash, sha256Bytes(csrf)) {
			return Principal{}, "", errors.New("csrf validation failed")
		}
	}
	return p, cookie.Value, nil
}

func (s *SessionStore) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE admin_user_session SET revoked_at = CURRENT_TIMESTAMP(3) WHERE token_hash = $1 AND revoked_at IS NULL`, sha256Bytes(token))
	return err
}

func (s *SessionStore) RevokeUserSessions(ctx context.Context, orgID, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE admin_user_session SET revoked_at = CURRENT_TIMESTAMP(3) WHERE org_id = $1 AND user_id = $2 AND revoked_at IS NULL`, orgID, userID)
	return err
}

func (s *SessionStore) SetCookies(w http.ResponseWriter, result LoginResult) {
	s.SetLoginCookies(w, result, result.SessionToken)
}

// SetLoginCookies is separate so the raw session token never needs to be kept in Principal.
func (s *SessionStore) SetLoginCookies(w http.ResponseWriter, result LoginResult, token string) {
	maxAge := int(s.sessionTTL.Seconds())
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
	http.SetCookie(w, &http.Cookie{Name: CSRFCookieName, Value: result.CSRFToken, Path: "/", HttpOnly: false, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func (s *SessionStore) ClearCookies(w http.ResponseWriter) {
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == SessionCookieName, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode})
	}
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sha256Bytes(v string) []byte { sum := sha256.Sum256([]byte(v)); return sum[:] }

const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // KiB; RFC 9106's lower-memory recommendation.
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// HashPassword uses the official golang.org/x/crypto Argon2id implementation.
// The encoded format keeps parameters with the hash so future cost increases can
// be rolled out through NeedsPasswordRehash and the next successful login.
func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", errors.New("password must contain at least 12 characters")
	}
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argon2Memory, argon2Time, argon2Threads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived)), nil
}

func VerifyPassword(password, encoded string) bool {
	if strings.HasPrefix(encoded, "argon2id$") {
		return verifyArgon2id(password, encoded)
	}
	return verifyLegacyPBKDF2(password, encoded)
}

func NeedsPasswordRehash(encoded string) bool {
	return !strings.HasPrefix(encoded, fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$", argon2Memory, argon2Time, argon2Threads))
}

func verifyArgon2id(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return false
	}
	var memory, passes, threads uint32
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil || memory == 0 || passes == 0 || threads == 0 || threads > 255 {
		return false
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[3])
	want, hashErr := base64.RawStdEncoding.DecodeString(parts[4])
	if saltErr != nil || hashErr != nil || len(salt) < 16 || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, passes, memory, uint8(threads), uint32(len(want)))
	return hmac.Equal(got, want)
}

// verifyLegacyPBKDF2 is retained only to lazily migrate hashes written by the
// previous local implementation. New and changed passwords are always Argon2id.
func verifyLegacyPBKDF2(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	var iterations int
	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil || iterations < 100000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iterations, len(want))
	return hmac.Equal(got, want)
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	result := make([]byte, 0, keyLen)
	for block := 1; len(result) < keyLen; block++ {
		h := hmac.New(sha256.New, password)
		h.Write(salt)
		h.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := h.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			h = hmac.New(sha256.New, password)
			h.Write(u)
			u = h.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		result = append(result, t...)
	}
	return result[:keyLen]
}
