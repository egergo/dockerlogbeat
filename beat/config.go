package dockerlogbeat

type RootConfig struct {
	Config *Config `yaml:"dockerlogbeat"`
}

type Config struct {
	SpoolSize          uint64 `yaml:"spool_size"`
	IdleTimeout        string `yaml:"idle_timeout"`
	RegistryFile       string `yaml:"registry_file"`
	StopTutumLogrotate bool   `yaml:"stop_tutum_logrotate"`
}

func NewDefaultConfig() *RootConfig {
	return &RootConfig{
		Config: &Config{
			StopTutumLogrotate: true,
		},
	}
}
