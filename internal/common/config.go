package common

import (
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultRefreshInterval = 30 * time.Minute
	defaultPingInterval    = 30 * time.Second
	//defaultSendTimeout  = 3 * time.Minute
	defaultSyncDelay    = 1 * time.Minute
	defaultSyncInterval = 6 * time.Hour
)

type Configure struct {
	Limb struct {
		Account  int64  `yaml:"account"`
		Password string `yaml:"password"`
		Protocol int    `yaml:"protocol"`
		HookSelf bool   `yaml:"hook_self"`

		Sign struct {
			Server          string        `yaml:"server"`
			Bearer          string        `yaml:"bearer"`
			Key             string        `yaml:"key"`
			IsBelow110      bool          `yaml:"is_below_110"`
			RefreshInterval time.Duration `yaml:"refresh_interval"`
		} `yaml:"sign"`
	} `yaml:"limb"`

	Service struct {
		Addr         string        `yaml:"addr"`
		Secret       string        `yaml:"secret"`
		PingInterval time.Duration `yaml:"ping_interval"`
		//SendTiemout  time.Duration `yaml:"send_timeout"`
		SyncDelay    time.Duration `yaml:"sync_delay"`
		SyncInterval time.Duration `yaml:"sync_interval"`
	} `yaml:"service"`

	Log struct {
		Level string `yaml:"level"`
	} `yaml:"log"`
}

func LoadConfig(path string) (*Configure, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &Configure{}
	config.Limb.Sign.RefreshInterval = defaultRefreshInterval
	config.Service.PingInterval = defaultPingInterval
	//config.Service.SendTiemout = defaultSendTimeout
	config.Service.SyncDelay = defaultSyncDelay
	config.Service.SyncInterval = defaultSyncInterval
	if err := yaml.Unmarshal(file, &config); err != nil {
		return nil, err
	}

	if !strings.HasSuffix(config.Limb.Sign.Server, "/") {
		config.Limb.Sign.Server += "/"
	}

	return config, nil
}
