package authorization

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"sync"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
)

// EnforcerManager keeps one official Casbin Enforcer per tenant domain. The
// adapter loads only that tenant's p/g rules, so a policy from one org cannot
// accidentally satisfy a request in another org.
type EnforcerManager struct {
	db        *sql.DB
	mu        sync.RWMutex
	enforcers map[int64]*casbin.SyncedEnforcer
}

func NewEnforcerManager(db *sql.DB) *EnforcerManager {
	return &EnforcerManager{db: db, enforcers: make(map[int64]*casbin.SyncedEnforcer)}
}

func (m *EnforcerManager) get(ctx context.Context, orgID int64) (*casbin.SyncedEnforcer, error) {
	m.mu.RLock()
	e := m.enforcers[orgID]
	m.mu.RUnlock()
	if e != nil {
		return e, nil
	}
	model, err := casbinmodel.NewModelFromString(modelConfig)
	if err != nil {
		return nil, fmt.Errorf("load casbin model: %w", err)
	}
	e, err = casbin.NewSyncedEnforcer(model, NewPostgresAdapter(ctx, m.db, orgID))
	if err != nil {
		return nil, fmt.Errorf("create casbin enforcer for org %d: %w", orgID, err)
	}
	e.EnableAutoSave(false)
	m.mu.Lock()
	if existing := m.enforcers[orgID]; existing != nil {
		e = existing
	} else {
		m.enforcers[orgID] = e
	}
	m.mu.Unlock()
	return e, nil
}

func (m *EnforcerManager) Enforce(ctx context.Context, subject Subject, permissionCode string) (bool, error) {
	if subject.UserID <= 0 || subject.OrgID <= 0 {
		return false, nil
	}
	e, err := m.get(ctx, subject.OrgID)
	if err != nil {
		return false, err
	}
	return e.Enforce("user:"+strconv.FormatInt(subject.UserID, 10), Domain(subject.OrgID), permissionCode, "*")
}

func (m *EnforcerManager) InvalidateOrg(orgID int64) {
	m.mu.Lock()
	delete(m.enforcers, orgID)
	m.mu.Unlock()
}

func (m *EnforcerManager) InvalidateAll() {
	m.mu.Lock()
	m.enforcers = make(map[int64]*casbin.SyncedEnforcer)
	m.mu.Unlock()
}
