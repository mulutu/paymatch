package config

import (
	"bufio"
	"encoding/base64"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// --- minimal .env loader (no extra deps) ---
func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // silently ignore if .env doesn’t exist
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// KEY=VALUE (keep everything after first '=' as the value)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		// remove surrounding quotes if present
		v = strings.Trim(v, `"'`)
		_ = os.Setenv(k, v) // set into process env
	}
}

// ------------------------------------------------

type AppCfg struct{ Env, Port, BaseURL, CallbackBaseURL string }
type DBCfg struct{ DSN string }
type RedisCfg struct{ Addr string }

type SecurityCfg struct {
	AESKey          []byte
	RateLimitPerMin int
	AdminToken      string // <— NEW: used to guard onboarding APIs
}

type Cfg struct {
	App   AppCfg
	DB    DBCfg
	Redis RedisCfg
	Sec   SecurityCfg
}

func Load() Cfg {
	// 1) Load .env into process env (if file exists)
	loadDotenv(".env")

	// 2) Read from env via viper
	viper.AutomaticEnv()
	viper.SetDefault("APP_ENV", "sandbox")
	viper.SetDefault("APP_PORT", "8080")
	viper.SetDefault("RATE_LIMIT_PER_MIN", 300)
	viper.SetDefault("TZ", "Africa/Nairobi")
	viper.SetDefault("ADMIN_TOKEN", "")

	// Ensure TZ
	if tz := viper.GetString("TZ"); tz != "" {
		os.Setenv("TZ", tz)
	}

	// Decode AES key
	keyB64 := viper.GetString("AES_256_KEY_BASE64")
	key, err := base64.StdEncoding.DecodeString(keyB64)

	cfg := Cfg{
		App: AppCfg{
			Env:             viper.GetString("APP_ENV"),
			Port:            viper.GetString("APP_PORT"),
			BaseURL:         viper.GetString("APP_BASE_URL"),
			CallbackBaseURL: viper.GetString("CALLBACK_BASE_URL"),
		},
		DB:    DBCfg{DSN: viper.GetString("DB_DSN")},
		Redis: RedisCfg{Addr: viper.GetString("REDIS_ADDR")},
		Sec: SecurityCfg{
			AESKey:          key,
			RateLimitPerMin: viper.GetInt("RATE_LIMIT_PER_MIN"),
			AdminToken:      strings.TrimSpace(viper.GetString("ADMIN_TOKEN")),
		},
	}

	// 3) Fail fast on required settings
	if cfg.DB.DSN == "" {
		log.Fatal().Msg("DB_DSN is required")
	}
	if err != nil || len(cfg.Sec.AESKey) != 32 {
		log.Fatal().Msg("AES_256_KEY_BASE64 must be a valid 32-byte base64 key")
	}

	_ = time.Local // TZ set via env
	return cfg
}
