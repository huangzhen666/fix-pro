package workforce

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }

type Trade struct {
	ID          string `json:"id"`
	Code        string `json:"tradeCode"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sortOrder"`
	Status      string `json:"status"`
	Version     int    `json:"version"`
	SkillCount  int    `json:"skillCount"`
}
type Skill struct {
	ID          string `json:"id"`
	TradeID     string `json:"tradeId"`
	TradeName   string `json:"tradeName,omitempty"`
	Code        string `json:"skillCode"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sortOrder"`
	Status      string `json:"status"`
	Version     int    `json:"version"`
}
type WorkerMedia struct {
	ID          string    `json:"id"`
	MediaType   string    `json:"mediaType"`
	ContentType string    `json:"contentType"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"createdAt"`
}
type Worker struct {
	ID                 string         `json:"id"`
	WorkerNo           string         `json:"workerNo"`
	Username           string         `json:"username,omitempty"`
	DisplayName        string         `json:"displayName"`
	Mobile             string         `json:"mobile,omitempty"`
	MobileMasked       string         `json:"mobileMasked,omitempty"`
	JoinedOn           *string        `json:"joinedOn,omitempty"`
	Remark             string         `json:"remark"`
	Status             string         `json:"status"`
	MustChangePassword bool           `json:"mustChangePassword"`
	Version            int            `json:"version"`
	OpenWorkOrderCount int            `json:"openWorkOrderCount"`
	Trades             []string       `json:"trades,omitempty"`
	Skills             []string       `json:"skills,omitempty"`
	TradeIDs           []int64        `json:"tradeIds,omitempty"`
	SkillIDs           []int64        `json:"skillIds,omitempty"`
	Avatar             *WorkerMedia   `json:"avatar,omitempty"`
	Certificates       []WorkerMedia  `json:"certificates,omitempty"`
	History            []HistoryEntry `json:"history,omitempty"`
	InitialPassword    string         `json:"initialPassword,omitempty"`
}
type HistoryEntry struct {
	EventCode    string          `json:"eventCode"`
	OperatorName string          `json:"operatorName"`
	Before       json.RawMessage `json:"before"`
	After        json.RawMessage `json:"after"`
	Reason       string          `json:"reason,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}
type Candidate struct {
	Worker
	MatchedSkills        []string `json:"matchedSkills"`
	MatchedSkillCount    int      `json:"matchedSkillCount"`
	RequiredSkillCount   int      `json:"requiredSkillCount"`
	AllSkillsMatched     bool     `json:"allSkillsMatched"`
	AppointmentAvailable bool     `json:"appointmentAvailable"`
}
type WorkerWrite struct {
	DisplayName         string  `json:"displayName"`
	Mobile              string  `json:"mobile"`
	TradeIDs            []int64 `json:"tradeIds"`
	SkillIDs            []int64 `json:"skillIds"`
	JoinedOn            string  `json:"joinedOn"`
	Remark              string  `json:"remark"`
	Activate            bool    `json:"activate"`
	Version             int     `json:"version"`
	AvatarMediaID       int64   `json:"avatarMediaId"`
	CertificateMediaIDs []int64 `json:"certificateMediaIds"`
}
type DisableRequest struct {
	Reason          string `json:"reason"`
	WorkOrderPolicy string `json:"workOrderPolicy"`
	Version         int    `json:"version"`
}

func admin(p auth.Principal) error {
	if p.Role != "ADMIN" {
		return httpx.E("FORBIDDEN", "无权管理师傅", 403)
	}
	return nil
}
func mask(v string) string {
	if len(v) < 8 {
		return v
	}
	return v[:3] + "****" + v[len(v)-4:]
}
func workerMediaURL(p auth.Principal, id string) string {
	if p.Role == "WORKER" {
		return "/api/v1/worker/media/" + id + "/content"
	}
	return "/api/v1/admin/media/" + id + "/content"
}
func ids(v []int64) map[int64]bool {
	out := map[int64]bool{}
	for _, x := range v {
		if x > 0 {
			out[x] = true
		}
	}
	return out
}
func (s *Service) history(tx *sql.Tx, p auth.Principal, id int64, event string, before, after any, reason string) error {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	_, e := tx.Exec(`INSERT INTO worker_profile_history(org_id,worker_id,event_code,operator_type,operator_id,operator_name,before_json,after_json,reason) VALUES($1,$2,$3,'ADMIN',$4,$5,$6,$7,NULLIF($8,''))`, p.OrgID, id, event, p.SubjectID, p.Name, b, a, reason)
	return e
}

func (s *Service) Trades(ctx context.Context, p auth.Principal, status string) ([]Trade, error) {
	if e := admin(p); e != nil {
		return nil, e
	}
	q := `SELECT t.id,t.trade_code,t.name,COALESCE(t.description,''),t.sort_order,t.status,t.version,(SELECT COUNT(*) FROM worker_skill s WHERE s.org_id=t.org_id AND s.trade_id=t.id) FROM worker_trade t WHERE t.org_id=$1 AND ($2='' OR t.status=$2) ORDER BY t.sort_order,t.id`
	rows, e := s.db.QueryContext(ctx, q, p.OrgID, status)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Trade{}
	for rows.Next() {
		var id int64
		var x Trade
		if e = rows.Scan(&id, &x.Code, &x.Name, &x.Description, &x.SortOrder, &x.Status, &x.Version, &x.SkillCount); e != nil {
			return nil, e
		}
		x.ID = fmt.Sprint(id)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Service) Skills(ctx context.Context, p auth.Principal, tradeID int64, status, key string) ([]Skill, error) {
	if e := admin(p); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT k.id,k.trade_id,t.name,k.skill_code,k.name,COALESCE(k.description,''),k.sort_order,k.status,k.version FROM worker_skill k JOIN worker_trade t ON t.id=k.trade_id AND t.org_id=k.org_id WHERE k.org_id=$1 AND ($2=0 OR k.trade_id=$2) AND ($3='' OR k.status=$3) AND ($4='' OR k.name ILIKE $5 OR k.skill_code ILIKE $5) ORDER BY k.sort_order,k.id`, p.OrgID, tradeID, status, key, "%"+key+"%")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		var id, tid int64
		var x Skill
		if e = rows.Scan(&id, &tid, &x.TradeName, &x.Code, &x.Name, &x.Description, &x.SortOrder, &x.Status, &x.Version); e != nil {
			return nil, e
		}
		x.ID, x.TradeID = fmt.Sprint(id), fmt.Sprint(tid)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Service) CreateTrade(ctx context.Context, p auth.Principal, x Trade) (Trade, error) {
	if e := admin(p); e != nil {
		return x, e
	}
	x.Name = strings.TrimSpace(x.Name)
	if len([]rune(x.Name)) < 2 {
		return x, httpx.E("VALIDATION_ERROR", "工种名称不合法", 400)
	}
	x.Code = fmt.Sprintf("TR%s%06d", time.Now().UTC().Format("20060102"), time.Now().UnixNano()%1000000)
	var id int64
	var e error
	for {
		e = s.db.QueryRowContext(ctx, `INSERT INTO worker_trade(org_id,trade_code,name,description,sort_order) VALUES($1,$2,$3,$4,$5) ON CONFLICT (org_id,trade_code) DO NOTHING RETURNING id`, p.OrgID, x.Code, x.Name, x.Description, x.SortOrder).Scan(&id)
		if e == nil {
			break
		}
		if e == sql.ErrNoRows {
			x.Code = fmt.Sprintf("TR%s%06d", time.Now().UTC().Format("20060102"), time.Now().UnixNano()%1000000)
			continue
		}
		return x, httpx.E("TRADE_NAME_EXISTS", "工种名称已存在", 409)
	}
	x.ID = fmt.Sprint(id)
	x.Status = "ACTIVE"
	return x, nil
}
func (s *Service) UpdateTrade(ctx context.Context, p auth.Principal, id int64, x Trade) (Trade, error) {
	if e := admin(p); e != nil {
		return x, e
	}
	x.Name = strings.TrimSpace(x.Name)
	if len([]rune(x.Name)) < 2 || len([]rune(x.Name)) > 64 {
		return x, httpx.E("VALIDATION_ERROR", "工种名称需为 2-64 字", 400)
	}
	r, e := s.db.ExecContext(ctx, `UPDATE worker_trade SET name=$1,description=$2,sort_order=$3,version=version+1 WHERE org_id=$4 AND id=$5 AND version=$6`, x.Name, x.Description, x.SortOrder, p.OrgID, id, x.Version)
	if e != nil {
		return x, httpx.E("TRADE_NAME_EXISTS", "工种名称已存在", 409)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return x, httpx.E("RESOURCE_VERSION_CONFLICT", "工种已被修改", 409)
	}
	return x, nil
}
func (s *Service) SetTradeStatus(ctx context.Context, p auth.Principal, id int64, status string, version int) (Trade, error) {
	if e := admin(p); e != nil {
		return Trade{}, e
	}
	if status != "ACTIVE" && status != "DISABLED" {
		return Trade{}, httpx.E("VALIDATION_ERROR", "状态不合法", 400)
	}
	if status == "DISABLED" {
		var n int
		if e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM worker_skill WHERE org_id=$1 AND trade_id=$2 AND status='ACTIVE'`, p.OrgID, id).Scan(&n); e != nil {
			return Trade{}, e
		}
		if n > 0 {
			return Trade{}, httpx.E("TRADE_HAS_ACTIVE_SKILLS", "工种下存在启用技能", 409)
		}
	}
	r, e := s.db.ExecContext(ctx, `UPDATE worker_trade SET status=$1,version=version+1 WHERE org_id=$2 AND id=$3 AND version=$4`, status, p.OrgID, id, version)
	if e != nil {
		return Trade{}, e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return Trade{}, httpx.E("RESOURCE_VERSION_CONFLICT", "工种已被修改", 409)
	}
	return Trade{ID: fmt.Sprint(id), Status: status}, nil
}

func (s *Service) DeleteTrade(ctx context.Context, p auth.Principal, id int64) error {
	if e := admin(p); e != nil {
		return e
	}
	var skillCount, assignmentCount int
	if e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM worker_skill WHERE org_id=$1 AND trade_id=$2`, p.OrgID, id).Scan(&skillCount); e != nil {
		return e
	}
	if skillCount > 0 {
		return httpx.E("TRADE_HAS_SKILLS", "工种下仍有技能，请先删除全部技能", 409)
	}
	if e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM worker_trade_assignment WHERE org_id=$1 AND trade_id=$2`, p.OrgID, id).Scan(&assignmentCount); e != nil {
		return e
	}
	if assignmentCount > 0 {
		return httpx.E("TRADE_IN_USE", "工种已被师傅绑定，不能删除", 409)
	}
	r, e := s.db.ExecContext(ctx, `DELETE FROM worker_trade WHERE org_id=$1 AND id=$2`, p.OrgID, id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return httpx.E("TRADE_NOT_FOUND", "工种不存在", 404)
	}
	return nil
}
func (s *Service) CreateSkill(ctx context.Context, p auth.Principal, x Skill) (Skill, error) {
	if e := admin(p); e != nil {
		return x, e
	}
	x.Name = strings.TrimSpace(x.Name)
	tid := parse(x.TradeID)
	if tid == 0 || len([]rune(x.Name)) < 2 {
		return x, httpx.E("VALIDATION_ERROR", "技能名称或工种不合法", 400)
	}
	var tradeCode string
	if e := s.db.QueryRowContext(ctx, `SELECT trade_code FROM worker_trade WHERE org_id=$1 AND id=$2 AND status='ACTIVE'`, p.OrgID, tid).Scan(&tradeCode); e == sql.ErrNoRows {
		return x, httpx.E("WORKER_TRADE_INVALID", "工种不存在、已禁用或跨组织", 400)
	} else if e != nil {
		return x, e
	}
	var id int64
	for {
		x.Code = fmt.Sprintf("%s-SK%06d", tradeCode, time.Now().UnixNano()%1000000)
		e := s.db.QueryRowContext(ctx, `INSERT INTO worker_skill(org_id,trade_id,skill_code,name,description,sort_order) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (org_id,skill_code) DO NOTHING RETURNING id`, p.OrgID, tid, x.Code, x.Name, x.Description, x.SortOrder).Scan(&id)
		if e == nil {
			break
		}
		if e == sql.ErrNoRows {
			continue
		}
		return x, httpx.E("SKILL_NAME_EXISTS", "技能名称已存在或工种不可用", 409)
	}
	x.ID, x.Status = fmt.Sprint(id), "ACTIVE"
	return x, nil
}
func (s *Service) UpdateSkill(ctx context.Context, p auth.Principal, id int64, x Skill) (Skill, error) {
	if e := admin(p); e != nil {
		return x, e
	}
	x.Name = strings.TrimSpace(x.Name)
	if len([]rune(x.Name)) < 2 || len([]rune(x.Name)) > 64 {
		return x, httpx.E("VALIDATION_ERROR", "技能名称需为 2-64 字", 400)
	}
	r, e := s.db.ExecContext(ctx, `UPDATE worker_skill SET name=$1,description=$2,sort_order=$3,version=version+1 WHERE org_id=$4 AND id=$5 AND version=$6`, x.Name, x.Description, x.SortOrder, p.OrgID, id, x.Version)
	if e != nil {
		return x, e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return x, httpx.E("RESOURCE_VERSION_CONFLICT", "技能已被修改", 409)
	}
	return x, nil
}
func (s *Service) SetSkillStatus(ctx context.Context, p auth.Principal, id int64, status string, version int) (Skill, error) {
	if e := admin(p); e != nil {
		return Skill{}, e
	}
	if status != "ACTIVE" && status != "DISABLED" {
		return Skill{}, httpx.E("VALIDATION_ERROR", "状态不合法", 400)
	}
	r, e := s.db.ExecContext(ctx, `UPDATE worker_skill SET status=$1,version=version+1 WHERE org_id=$2 AND id=$3 AND version=$4`, status, p.OrgID, id, version)
	if e != nil {
		return Skill{}, e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return Skill{}, httpx.E("RESOURCE_VERSION_CONFLICT", "技能已被修改", 409)
	}
	return Skill{ID: fmt.Sprint(id), Status: status}, nil
}

func (s *Service) DeleteSkill(ctx context.Context, p auth.Principal, id int64) error {
	if e := admin(p); e != nil {
		return e
	}
	var assignmentCount, requirementCount int
	if e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM worker_skill_assignment WHERE org_id=$1 AND skill_id=$2`, p.OrgID, id).Scan(&assignmentCount); e != nil {
		return e
	}
	if e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM service_sku_skill_requirement WHERE org_id=$1 AND skill_id=$2`, p.OrgID, id).Scan(&requirementCount); e != nil {
		return e
	}
	if assignmentCount > 0 || requirementCount > 0 {
		return httpx.E("SKILL_IN_USE", "技能已被师傅或 SKU 使用，不能删除", 409)
	}
	r, e := s.db.ExecContext(ctx, `DELETE FROM worker_skill WHERE org_id=$1 AND id=$2`, p.OrgID, id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return httpx.E("SKILL_NOT_FOUND", "技能不存在", 404)
	}
	return nil
}

func (s *Service) worker(ctx context.Context, p auth.Principal, id int64) (Worker, error) {
	var x Worker
	var wid int64
	var joined sql.NullString
	var mobile string
	var avatarID sql.NullInt64
	e := s.db.QueryRowContext(ctx, `SELECT e.id,COALESCE(e.worker_no,''),e.username,e.display_name,COALESCE(e.mobile,''),e.joined_on::text,COALESCE(e.remark,''),e.status,e.version,e.avatar_media_id,(SELECT COUNT(*) FROM work_order w WHERE w.org_id=e.org_id AND w.assignee_id=e.id AND w.status NOT IN ('FINISHED','CANCELLED')) FROM employee_account e WHERE e.org_id=$1 AND e.id=$2 AND e.role='WORKER' AND e.deleted_at IS NULL`, p.OrgID, id).Scan(&wid, &x.WorkerNo, &x.Username, &x.DisplayName, &mobile, &joined, &x.Remark, &x.Status, &x.Version, &avatarID, &x.OpenWorkOrderCount)
	if e == sql.ErrNoRows {
		return x, httpx.E("WORKER_NOT_FOUND", "师傅不存在", 404)
	}
	if e != nil {
		return x, e
	}
	x.ID = fmt.Sprint(wid)
	x.Mobile = mobile
	x.MobileMasked = mask(mobile)
	_ = s.db.QueryRowContext(ctx, `SELECT must_change_password FROM employee_account WHERE org_id=$1 AND id=$2`, p.OrgID, wid).Scan(&x.MustChangePassword)
	if joined.Valid {
		x.JoinedOn = &joined.String
	}
	if avatarID.Valid {
		var media WorkerMedia
		var mediaID int64
		if e = s.db.QueryRowContext(ctx, `SELECT id,media_type,content_type,original_name,created_at FROM media_asset WHERE org_id=$1 AND id=$2 AND purpose='WORKER_AVATAR' AND status='READY'`, p.OrgID, avatarID.Int64).Scan(&mediaID, &media.MediaType, &media.ContentType, &media.Name, &media.CreatedAt); e == nil {
			media.ID = fmt.Sprint(mediaID)
			media.URL = workerMediaURL(p, media.ID)
			x.Avatar = &media
		} else if e != sql.ErrNoRows {
			return x, e
		}
	}
	rows, e := s.db.QueryContext(ctx, `SELECT a.trade_id,t.name FROM worker_trade_assignment a JOIN worker_trade t ON t.id=a.trade_id WHERE a.org_id=$1 AND a.worker_id=$2 ORDER BY t.sort_order,t.id`, p.OrgID, id)
	if e != nil {
		return x, e
	}
	for rows.Next() {
		var v string
		var tradeID int64
		rows.Scan(&tradeID, &v)
		x.TradeIDs = append(x.TradeIDs, tradeID)
		x.Trades = append(x.Trades, v)
	}
	rows.Close()
	rows, e = s.db.QueryContext(ctx, `SELECT a.skill_id,k.name FROM worker_skill_assignment a JOIN worker_skill k ON k.id=a.skill_id WHERE a.org_id=$1 AND a.worker_id=$2 ORDER BY k.sort_order,k.id`, p.OrgID, id)
	if e != nil {
		return x, e
	}
	for rows.Next() {
		var v string
		var skillID int64
		rows.Scan(&skillID, &v)
		x.SkillIDs = append(x.SkillIDs, skillID)
		x.Skills = append(x.Skills, v)
	}
	rows.Close()
	x.Certificates = []WorkerMedia{}
	certificateRows, e := s.db.QueryContext(ctx, `SELECT m.id,m.media_type,m.content_type,m.original_name,m.created_at FROM worker_certificate_media c JOIN media_asset m ON m.org_id=c.org_id AND m.id=c.media_id AND m.status='READY' WHERE c.org_id=$1 AND c.worker_id=$2 ORDER BY c.created_at,c.id`, p.OrgID, id)
	if e != nil {
		return x, e
	}
	for certificateRows.Next() {
		var media WorkerMedia
		var mediaID int64
		if e = certificateRows.Scan(&mediaID, &media.MediaType, &media.ContentType, &media.Name, &media.CreatedAt); e != nil {
			certificateRows.Close()
			return x, e
		}
		media.ID = fmt.Sprint(mediaID)
		media.URL = workerMediaURL(p, media.ID)
		x.Certificates = append(x.Certificates, media)
	}
	if e = certificateRows.Err(); e != nil {
		certificateRows.Close()
		return x, e
	}
	certificateRows.Close()
	historyRows, e := s.db.QueryContext(ctx, `SELECT event_code,operator_name,COALESCE(before_json,'null'::jsonb),COALESCE(after_json,'null'::jsonb),COALESCE(reason,''),created_at FROM worker_profile_history WHERE org_id=$1 AND worker_id=$2 ORDER BY created_at DESC,id DESC LIMIT 50`, p.OrgID, id)
	if e != nil {
		return x, e
	}
	defer historyRows.Close()
	for historyRows.Next() {
		var h HistoryEntry
		if e = historyRows.Scan(&h.EventCode, &h.OperatorName, &h.Before, &h.After, &h.Reason, &h.CreatedAt); e != nil {
			return x, e
		}
		x.History = append(x.History, h)
	}
	return x, nil
}
func (s *Service) Workers(ctx context.Context, p auth.Principal, status, key string) ([]Worker, error) {
	if e := admin(p); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT e.id FROM employee_account e WHERE e.org_id=$1 AND e.role='WORKER' AND e.deleted_at IS NULL AND ($2='' OR e.status=$2) AND ($3='' OR e.display_name ILIKE $4 OR e.worker_no ILIKE $4 OR e.mobile ILIKE $4) ORDER BY e.updated_at DESC,e.id DESC`, p.OrgID, status, key, "%"+key+"%")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Worker{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		x, e := s.worker(ctx, p, id)
		if e != nil {
			return nil, e
		}
		x.Mobile = ""
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Service) validateWorkerMedia(tx *sql.Tx, p auth.Principal, w WorkerWrite) error {
	if w.AvatarMediaID > 0 {
		var count int
		if e := tx.QueryRow(`SELECT COUNT(*) FROM media_asset WHERE org_id=$1 AND id=$2 AND purpose='WORKER_AVATAR' AND media_type='IMAGE' AND status='READY'`, p.OrgID, w.AvatarMediaID).Scan(&count); e != nil {
			return e
		}
		if count != 1 {
			return httpx.E("WORKER_AVATAR_INVALID", "师傅照片不存在或不可用", 400)
		}
	}
	seen := map[int64]bool{}
	for _, mediaID := range w.CertificateMediaIDs {
		if mediaID <= 0 || seen[mediaID] {
			return httpx.E("WORKER_CERTIFICATE_INVALID", "技能证书附件不合法", 400)
		}
		seen[mediaID] = true
	}
	if len(seen) > 0 {
		for mediaID := range seen {
			var count int
			if e := tx.QueryRow(`SELECT COUNT(*) FROM media_asset WHERE org_id=$1 AND id=$2 AND purpose='WORKER_CERTIFICATE' AND media_type='IMAGE' AND status='READY'`, p.OrgID, mediaID).Scan(&count); e != nil {
				return e
			}
			if count != 1 {
				return httpx.E("WORKER_CERTIFICATE_INVALID", "技能证书附件不存在或不可用", 400)
			}
		}
	}
	return nil
}
func (s *Service) replaceCertificateMedia(tx *sql.Tx, p auth.Principal, workerID int64, mediaIDs []int64) error {
	if _, e := tx.Exec(`DELETE FROM worker_certificate_media WHERE org_id=$1 AND worker_id=$2`, p.OrgID, workerID); e != nil {
		return e
	}
	for _, mediaID := range mediaIDs {
		if _, e := tx.Exec(`INSERT INTO worker_certificate_media(org_id,worker_id,media_id) VALUES($1,$2,$3)`, p.OrgID, workerID, mediaID); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) SaveWorker(ctx context.Context, p auth.Principal, id int64, w WorkerWrite) (Worker, error) {
	if e := admin(p); e != nil {
		return Worker{}, e
	}
	w.DisplayName = strings.TrimSpace(w.DisplayName)
	w.Mobile = strings.TrimSpace(w.Mobile)
	if len([]rune(w.DisplayName)) < 2 || !validWorkerMobile(w.Mobile) {
		return Worker{}, httpx.E("VALIDATION_ERROR", "姓名或手机号不合法", 400)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Worker{}, e
	}
	defer tx.Rollback()
	if e = s.validateWorkerMedia(tx, p, w); e != nil {
		return Worker{}, e
	}
	var workerID int64
	initialPassword := ""
	if id == 0 {
		initialPassword = "w" + w.Mobile
		initialHash, hashErr := auth.HashPassword("w" + w.Mobile)
		if hashErr != nil {
			return Worker{}, hashErr
		}
		no := fmt.Sprintf("WK%s%06d", time.Now().UTC().Format("20060102"), time.Now().UnixNano()%1000000)
		for {
			var exists bool
			if e = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM employee_account WHERE org_id=$1 AND worker_no=$2)`, p.OrgID, no).Scan(&exists); e != nil {
				return Worker{}, e
			}
			if !exists {
				break
			}
			no = fmt.Sprintf("WK%s%06d", time.Now().UTC().Format("20060102"), time.Now().UnixNano()%1000000)
		}
		e = tx.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,worker_no,username,display_name,password_hash,status,role,mobile,joined_on,remark,avatar_media_id,must_change_password,password_version) VALUES($1,$2,$3,$4,$5,'DRAFT','WORKER',$3,NULLIF($6,'')::date,$7,NULLIF($8,0),TRUE,1) RETURNING id`, p.OrgID, no, w.Mobile, w.DisplayName, initialHash, w.JoinedOn, w.Remark, w.AvatarMediaID).Scan(&workerID)
		if e != nil {
			return Worker{}, httpx.E("WORKER_MOBILE_EXISTS", "师傅编号或手机号已存在", 409)
		}
		if e = s.replaceAssignments(tx, p, workerID, w.TradeIDs, w.SkillIDs); e != nil {
			return Worker{}, e
		}
		if e = s.replaceCertificateMedia(tx, p, workerID, w.CertificateMediaIDs); e != nil {
			return Worker{}, e
		}
		if w.Activate {
			if e = s.validateEnable(tx, p, workerID); e != nil {
				return Worker{}, e
			}
			_, e = tx.Exec(`UPDATE employee_account SET status='ACTIVE',version=version+1 WHERE org_id=$1 AND id=$2`, p.OrgID, workerID)
			if e != nil {
				return Worker{}, e
			}
		}
		if e = s.history(tx, p, workerID, "CREATED", nil, w, ""); e != nil {
			return Worker{}, e
		}
	} else {
		workerID = id
		var oldMobile string
		if e = tx.QueryRowContext(ctx, `SELECT COALESCE(mobile,'') FROM employee_account WHERE org_id=$1 AND id=$2 AND role='WORKER' AND deleted_at IS NULL FOR UPDATE`, p.OrgID, id).Scan(&oldMobile); e == sql.ErrNoRows {
			return Worker{}, httpx.E("WORKER_NOT_FOUND", "师傅不存在", 404)
		} else if e != nil {
			return Worker{}, e
		}
		var duplicateMobile bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM employee_account WHERE org_id=$1 AND mobile=$2 AND id<>$3 AND role='WORKER' AND deleted_at IS NULL)`, p.OrgID, w.Mobile, id).Scan(&duplicateMobile); e != nil {
			return Worker{}, e
		}
		if duplicateMobile {
			return Worker{}, httpx.E("WORKER_MOBILE_EXISTS", "当前组织已有相同手机号的师傅", 409)
		}
		var initialHash string
		if oldMobile != w.Mobile {
			initialPassword = "w" + w.Mobile
			initialHash, e = auth.HashPassword("w" + w.Mobile)
			if e != nil {
				return Worker{}, e
			}
		}
		r, e := tx.Exec(`UPDATE employee_account SET username=$1,display_name=$2,mobile=$1,joined_on=NULLIF($3,'')::date,remark=$4,avatar_media_id=NULLIF($5,0),password_hash=CASE WHEN $6<>'' THEN $6 ELSE password_hash END,must_change_password=CASE WHEN $6<>'' THEN TRUE ELSE must_change_password END,password_version=CASE WHEN $6<>'' THEN password_version+1 ELSE password_version END,version=version+1 WHERE org_id=$7 AND id=$8 AND role='WORKER' AND version=$9`, w.Mobile, w.DisplayName, w.JoinedOn, w.Remark, w.AvatarMediaID, initialHash, p.OrgID, id, w.Version)
		if e != nil {
			return Worker{}, e
		}
		n, _ := r.RowsAffected()
		if n == 0 {
			return Worker{}, httpx.E("RESOURCE_VERSION_CONFLICT", "师傅已被修改", 409)
		}
		if e = s.replaceAssignments(tx, p, id, w.TradeIDs, w.SkillIDs); e != nil {
			return Worker{}, e
		}
		if e = s.replaceCertificateMedia(tx, p, id, w.CertificateMediaIDs); e != nil {
			return Worker{}, e
		}
		if w.Activate {
			if e = s.validateEnable(tx, p, id); e != nil {
				return Worker{}, e
			}
			_, e = tx.Exec(`UPDATE employee_account SET status='ACTIVE',version=version+1 WHERE org_id=$1 AND id=$2`, p.OrgID, id)
			if e != nil {
				return Worker{}, e
			}
		}
		if e = s.history(tx, p, id, "PROFILE_UPDATED", nil, w, ""); e != nil {
			return Worker{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return Worker{}, e
	}
	out, e := s.worker(ctx, p, workerID)
	if e != nil {
		return Worker{}, e
	}
	out.InitialPassword = initialPassword
	return out, nil
}

func (s *Service) ResetPassword(ctx context.Context, p auth.Principal, id int64, sessions *auth.WorkerSessionStore) (map[string]any, error) {
	if e := admin(p); e != nil {
		return nil, e
	}
	if sessions == nil {
		return nil, httpx.E("WORKER_AUTH_UNAVAILABLE", "师傅认证服务未初始化", 500)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var mobile string
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(mobile,'') FROM employee_account WHERE org_id=$1 AND id=$2 AND role='WORKER' AND deleted_at IS NULL FOR UPDATE`, p.OrgID, id).Scan(&mobile); e == sql.ErrNoRows {
		return nil, httpx.E("WORKER_NOT_FOUND", "师傅不存在", 404)
	} else if e != nil {
		return nil, e
	}
	if !validWorkerMobile(mobile) {
		return nil, httpx.E("WORKER_MOBILE_REQUIRED", "师傅手机号不合法，无法重置密码", 400)
	}
	temporaryPassword := "w" + mobile
	hash, e := auth.HashPassword(temporaryPassword)
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE employee_account SET password_hash=$1,must_change_password=TRUE,password_version=password_version+1,last_password_changed_at=CURRENT_TIMESTAMP(3),version=version+1 WHERE org_id=$2 AND id=$3`, hash, p.OrgID, id); e != nil {
		return nil, e
	}
	if e = s.history(tx, p, id, "PASSWORD_RESET", nil, map[string]any{"mustChangePassword": true}, "后台重置师傅密码"); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	if e = sessions.RevokeWorkerSessions(ctx, p.OrgID, id); e != nil {
		return nil, e
	}
	return map[string]any{"workerId": id, "temporaryPassword": temporaryPassword, "mustChangePassword": true}, nil
}
func (s *Service) replaceAssignments(tx *sql.Tx, p auth.Principal, id int64, trades, skills []int64) error {
	tm, sm := ids(trades), ids(skills)
	if len(tm) == 0 && len(sm) == 0 {
		return nil
	}
	for skill := range sm {
		var tid int64
		if e := tx.QueryRow(`SELECT trade_id FROM worker_skill WHERE org_id=$1 AND id=$2 AND status='ACTIVE'`, p.OrgID, skill).Scan(&tid); e != nil || !tm[tid] {
			return httpx.E("WORKER_SKILL_INVALID", "技能不存在、已禁用或不属于已选工种", 400)
		}
	}
	if _, e := tx.Exec(`DELETE FROM worker_trade_assignment WHERE org_id=$1 AND worker_id=$2`, p.OrgID, id); e != nil {
		return e
	}
	for trade := range tm {
		if _, e := tx.Exec(`INSERT INTO worker_trade_assignment(org_id,worker_id,trade_id) SELECT $1,$2,id FROM worker_trade WHERE org_id=$1 AND id=$3 AND status='ACTIVE'`, p.OrgID, id, trade); e != nil {
			return e
		}
	}
	if _, e := tx.Exec(`DELETE FROM worker_skill_assignment WHERE org_id=$1 AND worker_id=$2`, p.OrgID, id); e != nil {
		return e
	}
	for skill := range sm {
		if _, e := tx.Exec(`INSERT INTO worker_skill_assignment(org_id,worker_id,skill_id) SELECT $1,$2,id FROM worker_skill WHERE org_id=$1 AND id=$3 AND status='ACTIVE'`, p.OrgID, id, skill); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) validateEnable(tx *sql.Tx, p auth.Principal, id int64) error {
	var t, sk int
	if e := tx.QueryRow(`SELECT COUNT(*) FROM worker_trade_assignment WHERE org_id=$1 AND worker_id=$2`, p.OrgID, id).Scan(&t); e != nil {
		return e
	}
	if t == 0 {
		return httpx.E("WORKER_TRADE_REQUIRED", "启用时至少配置一个工种", 400)
	}
	if e := tx.QueryRow(`SELECT COUNT(*) FROM worker_skill_assignment a JOIN worker_skill k ON k.id=a.skill_id WHERE a.org_id=$1 AND a.worker_id=$2 AND k.status='ACTIVE'`, p.OrgID, id).Scan(&sk); e != nil {
		return e
	}
	if sk == 0 {
		return httpx.E("WORKER_SKILL_REQUIRED", "启用时至少配置一项技能", 400)
	}
	return nil
}
func (s *Service) SetStatus(ctx context.Context, p auth.Principal, id int64, status string, req DisableRequest) error {
	if e := admin(p); e != nil {
		return e
	}
	if status == "ACTIVE" {
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if e = s.validateEnable(tx, p, id); e != nil {
			return e
		}
		r, e := tx.Exec(`UPDATE employee_account SET status='ACTIVE',version=version+1 WHERE org_id=$1 AND id=$2 AND status='DISABLED' AND version=$3`, p.OrgID, id, req.Version)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n == 0 {
			return httpx.E("RESOURCE_VERSION_CONFLICT", "师傅已被修改", 409)
		}
		if e = s.history(tx, p, id, "ACTIVATED", nil, map[string]string{"status": "ACTIVE"}, ""); e != nil {
			return e
		}
		return tx.Commit()
	}
	if status != "DISABLED" || strings.TrimSpace(req.Reason) == "" {
		return httpx.E("VALIDATION_ERROR", "停用状态或原因不合法", 400)
	}
	if req.WorkOrderPolicy == "" {
		req.WorkOrderPolicy = "KEEP_ASSIGNMENTS"
	}
	if req.WorkOrderPolicy != "KEEP_ASSIGNMENTS" && req.WorkOrderPolicy != "RETURN_NOT_STARTED" {
		return httpx.E("VALIDATION_ERROR", "停用策略不合法", 400)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var old string
	if e = tx.QueryRow(`SELECT status FROM employee_account WHERE org_id=$1 AND id=$2 AND role='WORKER' FOR UPDATE`, p.OrgID, id).Scan(&old); e == sql.ErrNoRows {
		return httpx.E("WORKER_NOT_FOUND", "师傅不存在", 404)
	}
	if e != nil {
		return e
	}
	if old != "ACTIVE" {
		return httpx.E("WORKER_STATUS_CONFLICT", "当前状态不允许停用", 409)
	}
	if _, e = tx.Exec(`UPDATE employee_account SET status='DISABLED',version=version+1 WHERE org_id=$1 AND id=$2 AND version=$3`, p.OrgID, id, req.Version); e != nil {
		return e
	}
	if req.WorkOrderPolicy == "RETURN_NOT_STARTED" {
		pendingRows, queryErr := tx.Query(`SELECT id,status,version,appointment_at FROM work_order WHERE org_id=$1 AND assignee_id=$2 AND status IN ('PENDING_ACCEPT','PENDING_ARRIVAL') FOR UPDATE`, p.OrgID, id)
		if queryErr != nil {
			return queryErr
		}
		for pendingRows.Next() {
			var workID int64
			var workStatus string
			var workVersion int
			var appointment sql.NullTime
			if e = pendingRows.Scan(&workID, &workStatus, &workVersion, &appointment); e != nil {
				pendingRows.Close()
				return e
			}
			if _, e = tx.Exec(`UPDATE work_order SET status='PENDING_DISPATCH',assignee_id=NULL,version=version+1 WHERE org_id=$1 AND id=$2 AND version=$3`, p.OrgID, workID, workVersion); e != nil {
				pendingRows.Close()
				return e
			}
			if _, e = tx.Exec(`INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,'PENDING_DISPATCH','WORKER_DISABLED_RETURN','ADMIN',$4,$5,$6)`, p.OrgID, workID, workStatus, p.SubjectID, p.Name, req.Reason); e != nil {
				pendingRows.Close()
				return e
			}
			var ap any
			if appointment.Valid {
				ap = appointment.Time
			}
			if _, e = tx.Exec(`INSERT INTO work_order_assignment_history(org_id,work_order_id,from_assignee_id,to_assignee_id,from_appointment_at,to_appointment_at,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,NULL,$4,NULL,'WORKER_DISABLED_RETURN','ADMIN',$5,$6,$7)`, p.OrgID, workID, id, ap, p.SubjectID, p.Name, req.Reason); e != nil {
				pendingRows.Close()
				return e
			}
		}
		if e = pendingRows.Err(); e != nil {
			pendingRows.Close()
			return e
		}
		pendingRows.Close()
	}
	if e = s.history(tx, p, id, "DISABLED", map[string]string{"status": "ACTIVE"}, map[string]string{"status": "DISABLED"}, req.Reason); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Service) Candidates(ctx context.Context, p auth.Principal, workOrderID, tradeID, skillID int64) ([]Candidate, error) {
	if e := admin(p); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `WITH required AS (
        SELECT DISTINCT r.skill_id
        FROM work_order w
        JOIN work_order_item wi ON wi.org_id=w.org_id AND wi.work_order_id=w.id
        JOIN order_item oi ON oi.org_id=wi.org_id AND oi.id=wi.order_item_id
        JOIN service_sku_skill_requirement r ON r.org_id=oi.org_id AND r.sku_id=oi.sku_id
        WHERE w.org_id=$1 AND w.id=$2
    ), required_count AS (SELECT COUNT(*) AS total FROM required)
    SELECT e.id,COALESCE(e.worker_no,''),e.display_name,COALESCE(e.mobile,''),e.status,e.version,
           COUNT(DISTINCT w2.id),rc.total,
           COUNT(DISTINCT CASE WHEN req.skill_id IS NOT NULL THEN req.skill_id END),
           COALESCE((SELECT string_agg(t.name,'、' ORDER BY t.sort_order,t.id) FROM worker_trade_assignment ta JOIN worker_trade t ON t.org_id=ta.org_id AND t.id=ta.trade_id WHERE ta.org_id=e.org_id AND ta.worker_id=e.id),'') AS trade_names,
           COALESCE((SELECT string_agg(k.name,'、' ORDER BY k.sort_order,k.id) FROM worker_skill_assignment sa JOIN worker_skill k ON k.org_id=sa.org_id AND k.id=sa.skill_id WHERE sa.org_id=e.org_id AND sa.worker_id=e.id),'') AS skill_names,
           NOT EXISTS (SELECT 1 FROM work_order busy JOIN work_order target ON target.org_id=busy.org_id AND target.id=$2 WHERE busy.org_id=e.org_id AND busy.assignee_id=e.id AND busy.appointment_at::date=target.appointment_at::date AND busy.appointment_slot=target.appointment_slot AND busy.status NOT IN ('FINISHED','CANCELLED')) AS appointment_available
    FROM employee_account e CROSS JOIN required_count rc
    LEFT JOIN work_order w2 ON w2.org_id=e.org_id AND w2.assignee_id=e.id AND w2.status NOT IN ('FINISHED','CANCELLED')
    LEFT JOIN worker_skill_assignment wa ON wa.org_id=e.org_id AND wa.worker_id=e.id
    LEFT JOIN required req ON req.skill_id=wa.skill_id
    WHERE e.org_id=$1 AND e.role='WORKER' AND e.status='ACTIVE' AND e.deleted_at IS NULL
      AND ($3=0 OR EXISTS (SELECT 1 FROM worker_trade_assignment fta WHERE fta.org_id=e.org_id AND fta.worker_id=e.id AND fta.trade_id=$3))
      AND ($4=0 OR EXISTS (SELECT 1 FROM worker_skill_assignment fsa WHERE fsa.org_id=e.org_id AND fsa.worker_id=e.id AND fsa.skill_id=$4))
    GROUP BY e.id,e.worker_no,e.display_name,e.mobile,e.status,e.version,rc.total
    HAVING rc.total=0 OR COUNT(DISTINCT CASE WHEN req.skill_id IS NOT NULL THEN req.skill_id END)>0
    ORDER BY appointment_available DESC,COUNT(DISTINCT CASE WHEN req.skill_id IS NOT NULL THEN req.skill_id END) DESC,COUNT(DISTINCT w2.id),e.display_name`, p.OrgID, workOrderID, tradeID, skillID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var id int64
		var mobile string
		var x Candidate
		var tradeNames, skillNames string
		if e = rows.Scan(&id, &x.WorkerNo, &x.DisplayName, &mobile, &x.Status, &x.Version, &x.OpenWorkOrderCount, &x.RequiredSkillCount, &x.MatchedSkillCount, &tradeNames, &skillNames, &x.AppointmentAvailable); e != nil {
			return nil, e
		}
		if tradeNames != "" {
			x.Trades = strings.Split(tradeNames, "、")
		}
		if skillNames != "" {
			x.Skills = strings.Split(skillNames, "、")
		}
		x.ID = fmt.Sprint(id)
		x.MobileMasked = mask(mobile)
		x.AllSkillsMatched = x.RequiredSkillCount > 0 && x.MatchedSkillCount == x.RequiredSkillCount
		out = append(out, x)
	}
	return out, rows.Err()
}
func parse(v string) int64 { var n int64; fmt.Sscan(v, &n); return n }
