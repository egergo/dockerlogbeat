package dockerlogbeat

type Config struct {
	SpoolSize    uint64 `yaml:"spool_size"`
	IdleTimeout  string `yaml:"idle_timeout"`
	RegistryFile string `yaml:"registry_file"`
}
