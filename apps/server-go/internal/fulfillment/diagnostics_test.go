package fulfillment

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/fixpro/server/internal/platform/logging"
	"strings"
	"testing"
)

type diagnosticDriver struct{}
type diagnosticConn struct{ fail bool }
type diagnosticTx struct{ fail bool }

func (diagnosticDriver) Open(name string) (driver.Conn, error) {
	return diagnosticConn{fail: name == "fail"}, nil
}
func (diagnosticConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }
func (diagnosticConn) Close() error                        { return nil }
func (c diagnosticConn) Begin() (driver.Tx, error)         { return diagnosticTx{c.fail}, nil }
func (t diagnosticTx) Commit() error {
	if t.fail {
		return errors.New("commit failed")
	}
	return nil
}
func (diagnosticTx) Rollback() error { return nil }
func init()                          { sql.Register("diagnostic-commit-test", diagnosticDriver{}) }
func TestBusinessEventOnlyAfterCommit(t *testing.T) {
	for _, name := range []string{"fail", "success"} {
		t.Run(name, func(t *testing.T) {
			db, e := sql.Open("diagnostic-commit-test", name)
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			var b bytes.Buffer
			l, _ := logging.New(logging.Defaults(), &b)
			ctx := logging.Context(context.Background(), l, logging.Defaults(), "commit-req")
			tx, e := db.BeginTx(ctx, nil)
			if e != nil {
				t.Fatal(e)
			}
			e = committed(ctx, tx, "work_order.assigned", 1, 2, 3, "PENDING_DISPATCH", "PENDING_ACCEPT")
			if name == "fail" {
				if e == nil || b.Len() != 0 {
					t.Fatal("failed commit emitted success")
				}
			} else {
				if e != nil || !strings.Contains(b.String(), `"workOrderId":"2"`) || !strings.Contains(b.String(), `"orderId":"3"`) {
					t.Fatalf("missing successful association %s %v", b.String(), e)
				}
			}
		})
	}
}
