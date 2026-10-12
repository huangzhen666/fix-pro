package customer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"github.com/fixpro/server/internal/platform/logging"
	"github.com/fixpro/server/internal/platform/wechat"
	"go.uber.org/zap"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Profile struct {
	ID            string  `json:"id"`
	DisplayName   string  `json:"displayName"`
	Status        string  `json:"status"`
	MobileMasked  string  `json:"mobileMasked"`
	AvatarMediaID *string `json:"avatarMediaId"`
	AvatarURL     *string `json:"avatarUrl"`
	Version       int     `json:"version"`
}
type Session struct {
	Principal     auth.Principal
	AppID, OpenID string
	ExpiresAt     time.Time
}
type Service struct {
	DB      *sql.DB
	Gateway wechat.Gateway
	Codec   *MobileCodec
	OrgID   int64
	AppID   string
	TTL     time.Duration
}

func New(db *sql.DB, g wechat.Gateway, codec *MobileCodec, org int64, appid string, ttl time.Duration) *Service {
	if org <= 0 {
		org = 1
	}
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	return &Service{db, g, codec, org, appid, ttl}
}
func invalid() error {
	return httpx.E("CUSTOMER_SESSION_INVALID", "登录已失效，请重新授权手机号", 401)
}
func (s *Service) configured() error {
	if s.Codec == nil || s.AppID == "" {
		return httpx.E("PHONE_LOGIN_UNAVAILABLE", "手机号登录尚未配置，请联系平台", 503)
	}
	return nil
}
func (s *Service) Login(ctx context.Context, phoneCode, loginCode string) (any, error) {
	if e := s.configured(); e != nil {
		return nil, e
	}
	if strings.TrimSpace(phoneCode) == "" || strings.TrimSpace(loginCode) == "" || phoneCode == loginCode {
		return nil, httpx.E("VALIDATION_ERROR", "请提供独立的手机号授权和微信登录凭证", 400)
	}
	identity, e := s.Gateway.Identity(ctx, loginCode)
	if e != nil {
		return nil, e
	}
	phone, e := s.Gateway.Phone(ctx, phoneCode)
	if e != nil {
		return nil, e
	}
	mobile, e := NormalizePhone(phone.Number)
	if e != nil || phone.Country != "86" || phone.Watermark.AppID != s.AppID {
		return nil, httpx.E("PHONE_NUMBER_UNSUPPORTED", "暂仅支持中国大陆手机号", 400)
	}
	encrypted, e := s.Codec.Encrypt(mobile)
	if e != nil {
		return nil, e
	}
	hash := s.Codec.Hash(mobile)
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return nil, e
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	digest := sha256.Sum256([]byte(token))
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var ready bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM customer_initialization WHERE name='customer-auth-v1') AND EXISTS(SELECT 1 FROM organization WHERE id=$1 AND status='ACTIVE')`, s.OrgID).Scan(&ready)
	if e != nil {
		return nil, e
	}
	if !ready {
		return nil, httpx.E("PHONE_LOGIN_UNAVAILABLE", "客户账户初始化尚未完成", 503)
	}
	var id int64
	isNew := true
	nicknameBytes := make([]byte, 5)
	if _, e = rand.Read(nicknameBytes); e != nil {
		return nil, e
	}
	displayName := fmt.Sprintf("用户%x", nicknameBytes)
	e = tx.QueryRowContext(ctx, `INSERT INTO customer(org_id,display_name,mobile_cipher,mobile_hash,source_channel,status,last_login_at,last_activity_at,mobile_verified_at) VALUES($1,$4,$2,$3,'WECHAT_MINI','ACTIVE',now(),now(),now()) ON CONFLICT(org_id,mobile_hash) DO NOTHING RETURNING id`, s.OrgID, encrypted, hash, displayName).Scan(&id)
	if e == sql.ErrNoRows {
		isNew = false
		e = tx.QueryRowContext(ctx, `SELECT id FROM customer WHERE org_id=$1 AND mobile_hash=$2 FOR UPDATE`, s.OrgID, hash).Scan(&id)
	}
	if e != nil {
		return nil, e
	}
	var status string
	var deleted sql.NullTime
	e = tx.QueryRowContext(ctx, `SELECT status,deleted_at FROM customer WHERE org_id=$1 AND id=$2 FOR UPDATE`, s.OrgID, id).Scan(&status, &deleted)
	if e != nil {
		return nil, e
	}
	if status != "ACTIVE" || deleted.Valid {
		return nil, httpx.E("CUSTOMER_ACCOUNT_UNAVAILABLE", "账户暂不可用，请联系平台", 403)
	}
	_, e = tx.ExecContext(ctx, `UPDATE customer SET last_login_at=now(),last_activity_at=now(),mobile_verified_at=now() WHERE org_id=$1 AND id=$2`, s.OrgID, id)
	if e != nil {
		return nil, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO customer_wechat_identity(org_id,customer_id,appid,openid,unionid) VALUES($1,$2,$3,$4,NULLIF($5,'')) ON CONFLICT(org_id,customer_id,appid,openid) DO UPDATE SET updated_at=now(),unionid=EXCLUDED.unionid`, s.OrgID, id, s.AppID, identity.OpenID, identity.UnionID)
	if e != nil {
		return nil, e
	}
	expires := time.Now().UTC().Add(s.TTL)
	_, e = tx.ExecContext(ctx, `INSERT INTO customer_session(org_id,customer_id,appid,openid,token_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, s.OrgID, id, s.AppID, identity.OpenID, digest[:], expires)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	logging.Bind(ctx, zap.String("principalType", "CUSTOMER"), zap.String("principalId", fmt.Sprint(id)), zap.String("orgId", fmt.Sprint(s.OrgID)))
	logging.Event(ctx, "auth", "auth.customer_login")
	profile, e := s.Profile(ctx, s.OrgID, id)
	if e != nil {
		return nil, e
	}
	return map[string]any{"accessToken": token, "expiresAt": expires, "isNewCustomer": isNew, "customer": profile}, nil
}
func (s *Service) Session(r *http.Request) (Session, error) {
	var out Session
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return out, invalid()
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" || strings.HasPrefix(token, "local-") {
		return out, invalid()
	}
	hash := sha256.Sum256([]byte(token))
	e := s.DB.QueryRowContext(r.Context(), `SELECT cs.id,cs.org_id,cs.customer_id,cs.appid,cs.openid,cs.expires_at,c.display_name FROM customer_session cs JOIN customer c ON c.org_id=cs.org_id AND c.id=cs.customer_id JOIN organization o ON o.id=c.org_id WHERE cs.token_hash=$1 AND cs.revoked_at IS NULL AND cs.expires_at>now() AND c.status='ACTIVE' AND c.deleted_at IS NULL AND o.status='ACTIVE'`, hash[:]).Scan(&out.Principal.SessionID, &out.Principal.OrgID, &out.Principal.SubjectID, &out.AppID, &out.OpenID, &out.ExpiresAt, &out.Principal.Name)
	if e == sql.ErrNoRows {
		return out, invalid()
	}
	if e != nil {
		return out, e
	}
	if out.AppID != s.AppID || out.Principal.OrgID != s.OrgID {
		return out, invalid()
	}
	out.Principal.Role = "CUSTOMER"
	out.Principal.SessionAuthenticated = true
	return out, nil
}
func (s *Service) Authenticate(r *http.Request) (auth.Principal, error) {
	v, e := s.Session(r)
	return v.Principal, e
}
func (s *Service) Profile(ctx context.Context, org, id int64) (Profile, error) {
	var p Profile
	var n int64
	var mobile []byte
	var avatar sql.NullInt64
	e := s.DB.QueryRowContext(ctx, `SELECT id,display_name,status,mobile_cipher,avatar_media_id,version FROM customer WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL`, org, id).Scan(&n, &p.DisplayName, &p.Status, &mobile, &avatar, &p.Version)
	if e == sql.ErrNoRows {
		return p, httpx.E("CUSTOMER_NOT_FOUND", "用户不存在", 404)
	}
	if e != nil {
		return p, e
	}
	p.ID = fmt.Sprint(n)
	if s.Codec != nil && len(mobile) > 0 {
		v, e := s.Codec.Decrypt(mobile)
		if e != nil {
			return p, e
		}
		p.MobileMasked = MaskPhone(v)
	}
	if avatar.Valid {
		v := fmt.Sprint(avatar.Int64)
		u := "/api/v1/mini/media/" + v + "/content"
		p.AvatarMediaID = &v
		p.AvatarURL = &u
	}
	return p, nil
}

type ProfileWrite struct {
	DisplayName   string  `json:"displayName"`
	AvatarMediaID *string `json:"avatarMediaId"`
	Version       *int    `json:"version"`
}

func validateName(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) < 1 || utf8.RuneCountInString(v) > 32 {
		return "", httpx.E("VALIDATION_ERROR", "昵称需为 1 至 32 个字符", 400)
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", httpx.E("VALIDATION_ERROR", "昵称不能含控制字符或换行", 400)
		}
	}
	return v, nil
}
func (s *Service) UpdateProfile(ctx context.Context, session Session, w ProfileWrite) (Profile, error) {
	var zero Profile
	name, e := validateName(w.DisplayName)
	if e != nil {
		return zero, e
	}
	if w.Version == nil || *w.Version < 0 {
		return zero, httpx.E("VALIDATION_ERROR", "缺少资料版本", 400)
	}
	var oldName string
	if e = s.DB.QueryRowContext(ctx, `SELECT display_name FROM customer WHERE org_id=$1 AND id=$2`, session.Principal.OrgID, session.Principal.SubjectID).Scan(&oldName); e != nil {
		return zero, e
	}
	if name != oldName {
		if e = s.Gateway.CheckNickname(ctx, session.OpenID, name); e != nil {
			return zero, e
		}
	}
	var avatar any
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return zero, e
	}
	defer tx.Rollback()
	if w.AvatarMediaID != nil {
		var id int64
		_, e = fmt.Sscan(*w.AvatarMediaID, &id)
		if e != nil || fmt.Sprint(id) != *w.AvatarMediaID || id <= 0 {
			return zero, httpx.E("VALIDATION_ERROR", "头像编号无效", 400)
		}
		var found int64
		e = tx.QueryRowContext(ctx, `SELECT id FROM media_asset WHERE id=$1 AND org_id=$2 AND owner_type='CUSTOMER' AND owner_id=$3 AND purpose='CUSTOMER_AVATAR' AND status='READY' FOR UPDATE`, id, session.Principal.OrgID, session.Principal.SubjectID).Scan(&found)
		if e == sql.ErrNoRows {
			return zero, httpx.E("MEDIA_NOT_FOUND", "头像不可用", 404)
		}
		if e != nil {
			return zero, e
		}
		avatar = id
	}
	result, e := tx.ExecContext(ctx, `UPDATE customer SET display_name=$1,avatar_media_id=$2,version=version+1 WHERE org_id=$3 AND id=$4 AND version=$5 AND status='ACTIVE' AND deleted_at IS NULL`, name, avatar, session.Principal.OrgID, session.Principal.SubjectID, *w.Version)
	if e != nil {
		return zero, e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return zero, httpx.E("RESOURCE_VERSION_CONFLICT", "资料已更新，请重新载入后编辑", 409)
	}
	if e = tx.Commit(); e != nil {
		return zero, e
	}
	return s.Profile(ctx, session.Principal.OrgID, session.Principal.SubjectID)
}
func (s *Service) Touch(ctx context.Context, p auth.Principal) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE customer SET last_activity_at=now() WHERE org_id=$1 AND id=$2 AND (last_activity_at IS NULL OR last_activity_at<now()-interval '1 minute')`, p.OrgID, p.SubjectID)
}
