package linuxpatch

import (
	"os"
	"strings"
	"time"
)

// Config holds SSH defaults for the Linux Patch agent.
type Config struct {
	SSHUser    string
	SSHKeyPath string
	SSHKeyPEM  string // raw PEM from env (optional)
	SSHPassword string
	Timeout    time.Duration
	KnownHosts string
}

// LoadConfig from environment.
func LoadConfig() Config {
	t := 60 * time.Second
	return Config{
		SSHUser:     first(os.Getenv("LINUX_PATCH_SSH_USER"), os.Getenv("SSH_USER"), "root"),
		SSHKeyPath:  first(os.Getenv("LINUX_PATCH_SSH_KEY"), os.Getenv("SSH_PRIVATE_KEY_PATH")),
		SSHKeyPEM:   strings.TrimSpace(os.Getenv("LINUX_PATCH_SSH_KEY_PEM")),
		SSHPassword: os.Getenv("LINUX_PATCH_SSH_PASSWORD"),
		Timeout:     t,
		KnownHosts:  os.Getenv("LINUX_PATCH_KNOWN_HOSTS"),
	}
}

func (c Config) HasAuth() bool {
	return c.SSHKeyPath != "" || c.SSHKeyPEM != "" || c.SSHPassword != ""
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
