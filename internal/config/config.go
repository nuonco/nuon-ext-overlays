package config

import "os"

type Config struct {
	APIURL     string
	APIToken   string
	OrgID      string
	AppID      string
	InstallID  string
	ConfigFile string
	ExtName    string
	ExtDir     string
}

func Load() *Config {
	cfg := &Config{
		APIURL:     os.Getenv("NUON_API_URL"),
		APIToken:   os.Getenv("NUON_API_TOKEN"),
		OrgID:      os.Getenv("NUON_ORG_ID"),
		AppID:      os.Getenv("NUON_APP_ID"),
		InstallID:  os.Getenv("NUON_INSTALL_ID"),
		ConfigFile: os.Getenv("NUON_CONFIG_FILE"),
		ExtName:    os.Getenv("NUON_EXT_NAME"),
		ExtDir:     os.Getenv("NUON_EXT_DIR"),
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "https://api.nuon.co"
	}
	return cfg
}
