package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
)

func main() {
	orgID := flag.Int64("org-id", 1, "organization id")
	username := flag.String("username", "admin", "admin username")
	displayName := flag.String("display-name", "平台管理员", "display name")
	password := flag.String("password", "", "initial password; omitted means generate one")
	platform := flag.Bool("platform-super-admin", false, "grant platform super admin")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	c, err := config.Load()
	if err != nil {
		log.Error("config", "error", err)
		os.Exit(1)
	}
	db, err := database.Open(context.Background(), c)
	if err != nil {
		log.Error("database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	pass := strings.TrimSpace(*password)
	if pass == "" {
		b := make([]byte, 18)
		if _, err := rand.Read(b); err != nil {
			log.Error("password", "error", err)
			os.Exit(1)
		}
		pass = "Fx!" + base64.RawURLEncoding.EncodeToString(b)
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		log.Error("password", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Error("transaction", "error", err)
		os.Exit(1)
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO admin_user(org_id,username,display_name,password_hash,must_change_password) VALUES($1,$2,$3,$4,TRUE) ON CONFLICT (org_id,username) DO UPDATE SET display_name=EXCLUDED.display_name, password_hash=EXCLUDED.password_hash, status='ACTIVE', must_change_password=TRUE, version=admin_user.version+1 RETURNING id`, *orgID, strings.TrimSpace(*username), strings.TrimSpace(*displayName), hash).Scan(&id)
	if err != nil {
		log.Error("admin user", "error", err)
		os.Exit(1)
	}
	if *platform {
		if _, err = tx.ExecContext(ctx, `INSERT INTO admin_platform_super_admin(user_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
			log.Error("platform admin", "error", err)
			os.Exit(1)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_user_role(org_id,user_id,role_id) SELECT $1,$2,id FROM admin_role WHERE org_id=$1 AND role_code='tenant_admin' ON CONFLICT DO NOTHING`, *orgID, id); err != nil {
		log.Error("tenant role", "error", err)
		os.Exit(1)
	}
	if err = tx.Commit(); err != nil {
		log.Error("commit", "error", err)
		os.Exit(1)
	}
	fmt.Printf("管理员初始化完成\n用户: %s\n初始密码（仅本次显示）: %s\n请登录后立即修改密码。\n", *username, pass)
}
