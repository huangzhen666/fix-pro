package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                string
	HTTPAddr           string
	DBDSN              string
	DBMaxOpen          int
	DBMaxIdle          int
	MediaDriver        string
	MediaLocalRoot     string
	AdminUsername      string
	AdminPassword      string
	CORSAllowedOrigins []string
	ShutdownTimeout    time.Duration
}

func Load() (Config, error) {
	c := Config{
		Env: env("APP_ENV", "local"), HTTPAddr: env("HTTP_ADDR", ":8080"),
		DBDSN:     env("DB_DSN", "postgres://fixpro:fixpro-local@localhost:5432/fix_pro?sslmode=disable&timezone=UTC"),
		DBMaxOpen: envInt("DB_MAX_OPEN_CONNS", 20), DBMaxIdle: envInt("DB_MAX_IDLE_CONNS", 5),
		MediaDriver: env("MEDIA_DRIVER", "local"), MediaLocalRoot: env("MEDIA_LOCAL_ROOT", os.TempDir()+"/fixpro-media"),
		AdminUsername: env("APP_ADMIN_USERNAME", "admin"), AdminPassword: env("APP_ADMIN_PASSWORD", "change-me-in-production"),
		CORSAllowedOrigins: split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")), ShutdownTimeout: 15 * time.Second,
	}
	if c.DBDSN == "" {
		return Config{}, errors.New("DB_DSN is required")
	}
	if c.Env == "production" {
		if c.AdminPassword == "change-me-in-production" {
			return Config{}, errors.New("production default admin password is forbidden")
		}
		if c.MediaDriver == "local" {
			return Config{}, errors.New("production local media driver is forbidden")
		}
		if strings.Contains(strings.ToLower(c.DBDSN), "sslmode=disable") {
			return Config{}, errors.New("production database TLS cannot be disabled")
		}
	}
	if c.MediaDriver != "local" && c.MediaDriver != "s3" {
		return Config{}, errors.New("MEDIA_DRIVER must be local or s3")
	}
	return c, nil
}

func env(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}
func envInt(k string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(k))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
func split(v string) []string {
	out := []string{}
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
