package address

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }

type Write struct {
	City          string `json:"city"`
	DetailAddress string `json:"detailAddress"`
	BuildingDoor  string `json:"buildingDoor"`
	ContactName   string `json:"contactName"`
	ContactMobile string `json:"contactMobile"`
	IsDefault     bool   `json:"isDefault"`
}

type Address struct {
	ID            string    `json:"id"`
	City          string    `json:"city"`
	DetailAddress string    `json:"detailAddress"`
	BuildingDoor  string    `json:"buildingDoor"`
	ContactName   string    `json:"contactName"`
	ContactMobile string    `json:"contactMobile"`
	IsDefault     bool      `json:"isDefault"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

var mobile = regexp.MustCompile(`^1\d{10}$`)

func validate(w Write) error {
	if n := len([]rune(strings.TrimSpace(w.City))); n < 2 || n > 64 {
		return httpx.E("VALIDATION_ERROR", "所在城市需为 2-64 字", 400)
	}
	if n := len([]rune(strings.TrimSpace(w.DetailAddress))); n < 2 || n > 255 {
		return httpx.E("VALIDATION_ERROR", "具体地址需为 2-255 字", 400)
	}
	if n := len([]rune(strings.TrimSpace(w.BuildingDoor))); n < 1 || n > 128 {
		return httpx.E("VALIDATION_ERROR", "楼号/门牌号不能为空且不超过128字", 400)
	}
	if n := len([]rune(strings.TrimSpace(w.ContactName))); n < 2 || n > 64 {
		return httpx.E("VALIDATION_ERROR", "联系人需为 2-64 字", 400)
	}
	if !mobile.MatchString(strings.TrimSpace(w.ContactMobile)) {
		return httpx.E("INVALID_CONTACT_MOBILE", "手机号格式错误", 400)
	}
	return nil
}

func normalized(w Write) Write {
	w.City = strings.TrimSpace(w.City)
	w.DetailAddress = strings.TrimSpace(w.DetailAddress)
	w.BuildingDoor = strings.TrimSpace(w.BuildingDoor)
	w.ContactName = strings.TrimSpace(w.ContactName)
	w.ContactMobile = strings.TrimSpace(w.ContactMobile)
	return w
}

func (s *Service) List(ctx context.Context, p auth.Principal) ([]Address, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,city,detail_address,building_door,contact_name,contact_mobile,is_default,created_at,updated_at FROM customer_address WHERE org_id=$1 AND customer_id=$2 ORDER BY is_default DESC,updated_at DESC,id DESC`, p.OrgID, p.SubjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Address{}
	for rows.Next() {
		var id int64
		var item Address
		if err = rows.Scan(&id, &item.City, &item.DetailAddress, &item.BuildingDoor, &item.ContactName, &item.ContactMobile, &item.IsDefault, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ID = fmt.Sprint(id)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) Create(ctx context.Context, p auth.Principal, w Write) (Address, error) {
	w = normalized(w)
	if err := validate(w); err != nil {
		return Address{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Address{}, err
	}
	defer tx.Rollback()
	if err = lockCustomer(ctx, tx, p); err != nil {
		return Address{}, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM customer_address WHERE org_id=$1 AND customer_id=$2`, p.OrgID, p.SubjectID).Scan(&count); err != nil {
		return Address{}, err
	}
	w.IsDefault = w.IsDefault || count == 0
	if w.IsDefault {
		if _, err = tx.ExecContext(ctx, `UPDATE customer_address SET is_default=false WHERE org_id=$1 AND customer_id=$2`, p.OrgID, p.SubjectID); err != nil {
			return Address{}, err
		}
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO customer_address(org_id,customer_id,city,detail_address,building_door,contact_name,contact_mobile,is_default) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, p.OrgID, p.SubjectID, w.City, w.DetailAddress, w.BuildingDoor, w.ContactName, w.ContactMobile, w.IsDefault).Scan(&id)
	if err != nil {
		return Address{}, err
	}
	if err = tx.Commit(); err != nil {
		return Address{}, err
	}
	return Address{ID: fmt.Sprint(id), City: w.City, DetailAddress: w.DetailAddress, BuildingDoor: w.BuildingDoor, ContactName: w.ContactName, ContactMobile: w.ContactMobile, IsDefault: w.IsDefault, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, nil
}

func (s *Service) Update(ctx context.Context, p auth.Principal, id int64, w Write) (Address, error) {
	w = normalized(w)
	if err := validate(w); err != nil {
		return Address{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Address{}, err
	}
	defer tx.Rollback()
	if err = lockCustomer(ctx, tx, p); err != nil {
		return Address{}, err
	}
	var oldDefault bool
	if err = tx.QueryRowContext(ctx, `SELECT is_default FROM customer_address WHERE org_id=$1 AND customer_id=$2 AND id=$3 FOR UPDATE`, p.OrgID, p.SubjectID, id).Scan(&oldDefault); err == sql.ErrNoRows {
		return Address{}, httpx.E("ADDRESS_NOT_FOUND", "地址不存在", 404)
	} else if err != nil {
		return Address{}, err
	}
	if w.IsDefault {
		if _, err = tx.ExecContext(ctx, `UPDATE customer_address SET is_default=false WHERE org_id=$1 AND customer_id=$2 AND id<>$3`, p.OrgID, p.SubjectID, id); err != nil {
			return Address{}, err
		}
	} else if oldDefault {
		var other int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM customer_address WHERE org_id=$1 AND customer_id=$2 AND id<>$3 ORDER BY id LIMIT 1`, p.OrgID, p.SubjectID, id).Scan(&other)
		if err == sql.ErrNoRows {
			w.IsDefault = true
		} else if err != nil {
			return Address{}, err
		} else {
			if _, err = tx.ExecContext(ctx, `UPDATE customer_address SET is_default=false WHERE org_id=$1 AND customer_id=$2 AND id=$3`, p.OrgID, p.SubjectID, id); err != nil {
				return Address{}, err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE customer_address SET is_default=true WHERE org_id=$1 AND customer_id=$2 AND id=$3`, p.OrgID, p.SubjectID, other); err != nil {
				return Address{}, err
			}
		}
	}
	var item Address
	err = tx.QueryRowContext(ctx, `UPDATE customer_address SET city=$1,detail_address=$2,building_door=$3,contact_name=$4,contact_mobile=$5,is_default=$6 WHERE org_id=$7 AND customer_id=$8 AND id=$9 RETURNING id,city,detail_address,building_door,contact_name,contact_mobile,is_default,created_at,updated_at`, w.City, w.DetailAddress, w.BuildingDoor, w.ContactName, w.ContactMobile, w.IsDefault, p.OrgID, p.SubjectID, id).Scan(&id, &item.City, &item.DetailAddress, &item.BuildingDoor, &item.ContactName, &item.ContactMobile, &item.IsDefault, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Address{}, err
	}
	if err = tx.Commit(); err != nil {
		return Address{}, err
	}
	item.ID = fmt.Sprint(id)
	return item, nil
}

func (s *Service) SetDefault(ctx context.Context, p auth.Principal, id int64) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockCustomer(ctx, tx, p); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM customer_address WHERE org_id=$1 AND customer_id=$2 AND id=$3)`, p.OrgID, p.SubjectID, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return httpx.E("ADDRESS_NOT_FOUND", "地址不存在", 404)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE customer_address SET is_default=false WHERE org_id=$1 AND customer_id=$2`, p.OrgID, p.SubjectID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE customer_address SET is_default=true WHERE org_id=$1 AND customer_id=$2 AND id=$3`, p.OrgID, p.SubjectID, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Delete(ctx context.Context, p auth.Principal, id int64) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockCustomer(ctx, tx, p); err != nil {
		return err
	}
	var oldDefault bool
	if err = tx.QueryRowContext(ctx, `SELECT is_default FROM customer_address WHERE org_id=$1 AND customer_id=$2 AND id=$3 FOR UPDATE`, p.OrgID, p.SubjectID, id).Scan(&oldDefault); err == sql.ErrNoRows {
		return httpx.E("ADDRESS_NOT_FOUND", "地址不存在", 404)
	} else if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM customer_address WHERE org_id=$1 AND customer_id=$2 AND id=$3`, p.OrgID, p.SubjectID, id); err != nil {
		return err
	}
	if oldDefault {
		if _, err = tx.ExecContext(ctx, `UPDATE customer_address SET is_default=true WHERE id=(SELECT id FROM customer_address WHERE org_id=$1 AND customer_id=$2 ORDER BY id DESC LIMIT 1)`, p.OrgID, p.SubjectID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func lockCustomer(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM customer WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, p.OrgID, p.SubjectID).Scan(&id); err == sql.ErrNoRows {
		return httpx.E("CUSTOMER_NOT_FOUND", "客户不存在", 404)
	} else {
		return err
	}
}

type Handler struct{ s *Service }

func NewHandler(s *Service) *Handler { return &Handler{s: s} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	v, err := h.s.List(r.Context(), p)
	send(w, r, v, err)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var body Write
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	v, err := h.s.Create(r.Context(), p, body)
	send(w, r, v, err)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var body Write
	if err = httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	v, err := h.s.Update(r.Context(), p, id, body)
	send(w, r, v, err)
}

func (h *Handler) SetDefault(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err == nil {
		err = h.s.SetDefault(r.Context(), p, id)
	}
	send(w, r, map[string]bool{"ok": err == nil}, err)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err == nil {
		err = h.s.Delete(r.Context(), p, id)
	}
	send(w, r, map[string]bool{"ok": err == nil}, err)
}

func send(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		httpx.Failure(w, r, err)
	} else {
		httpx.Success(w, r, value)
	}
}
