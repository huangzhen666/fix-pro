package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
	"os"
	"strings"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
)

func main() { os.Exit(run()) }
func run() int {
	orgID := flag.Int64("org-id", 1, "organization id")
	username := flag.String("username", "admin", "admin username")
	displayName := flag.String("display-name", "平台管理员", "display name")
	password := flag.String("password", "", "initial password; omitted means generate one")
	passwordOutput := flag.String("password-output", "", "private new file for generated password when no terminal")
	platform := flag.Bool("platform-super-admin", false, "grant platform super admin")
	flag.Parse()
	log := logging.Bootstrap("fix-pro-bootstrap-admin")
	defer func() { _ = log.Sync() }()
	c, err := config.Load()
	if err != nil {
		log.Error("config", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "config"))...)
		return 1
	}
	c.Logging.Service = "fix-pro-bootstrap-admin"
	configured, logErr := logging.New(c.Logging, nil)
	if logErr != nil {
		log.Error("invalid log configuration", append(logging.SafeError(logErr), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "invalid log configuration"))...)
		return 1
	}
	log = configured
	db, err := database.Open(context.Background(), c)
	if err != nil {
		log.Error("database", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "database"))...)
		return 1
	}
	defer db.Close()
	pass := strings.TrimSpace(*password)
	if pass == "" {
		pass = os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	}
	var secretOutput *os.File
	if pass == "" {
		if *passwordOutput != "" {
			secretOutput, err = os.OpenFile(*passwordOutput, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		} else {
			secretOutput, err = os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		}
		if err != nil {
			log.Error("generated password requires terminal or private output file", logging.SafeError(err)...)
			return 1
		}
		defer secretOutput.Close()
	}

	if pass == "" {
		b := make([]byte, 18)
		if _, err := rand.Read(b); err != nil {
			log.Error("password", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "password"))...)
			return 1
		}
		pass = "Fx!" + base64.RawURLEncoding.EncodeToString(b)
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		log.Error("password", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "password"))...)
		return 1
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Error("transaction", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "transaction"))...)
		return 1
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO admin_user(org_id,username,display_name,password_hash,must_change_password) VALUES($1,$2,$3,$4,TRUE) ON CONFLICT (org_id,username) DO UPDATE SET display_name=EXCLUDED.display_name, password_hash=EXCLUDED.password_hash, status='ACTIVE', must_change_password=TRUE, version=admin_user.version+1 RETURNING id`, *orgID, strings.TrimSpace(*username), strings.TrimSpace(*displayName), hash).Scan(&id)
	if err != nil {
		log.Error("admin user", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "admin user"))...)
		return 1
	}
	if *platform {
		if _, err = tx.ExecContext(ctx, `INSERT INTO admin_platform_super_admin(user_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
			log.Error("platform admin", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "platform admin"))...)
			return 1
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_user_role(org_id,user_id,role_id) SELECT $1,$2,id FROM admin_role WHERE org_id=$1 AND role_code='tenant_admin' ON CONFLICT DO NOTHING`, *orgID, id); err != nil {
		log.Error("tenant role", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "tenant role"))...)
		return 1
	}
	if err = tx.Commit(); err != nil {
		log.Error("commit", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "commit"))...)
		return 1
	}
	if secretOutput != nil {
		if _, err = fmt.Fprintf(secretOutput, "初始密码：%s\n", pass); err != nil {
			log.Error("private password output failed", logging.SafeError(err)...)
			return 1
		}
	}
	log.Info("administrator initialized", zap.String("module", "maintenance"), zap.String("event", "maintenance.admin_initialized"), zap.String("principalId", fmt.Sprint(id)))
	return 0
}
