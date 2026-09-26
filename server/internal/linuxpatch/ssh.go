package linuxpatch

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// HostTarget is a parsed SSH destination.
type HostTarget struct {
	User string
	Host string
	Port string
	Raw  string
}

// ParseHosts splits comma/newline host list into targets.
func ParseHosts(raw, defaultUser string) []HostTarget {
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]HostTarget, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		t := HostTarget{User: defaultUser, Port: "22", Raw: p, Host: p}
		if at := strings.LastIndex(p, "@"); at >= 0 {
			t.User = p[:at]
			t.Host = p[at+1:]
		}
		if h, port, err := net.SplitHostPort(t.Host); err == nil {
			t.Host, t.Port = h, port
		} else if strings.Count(t.Host, ":") == 1 && !strings.Contains(t.Host, "]") {
			// host:port without brackets
			hp := strings.SplitN(t.Host, ":", 2)
			t.Host, t.Port = hp[0], hp[1]
		}
		if t.User == "" {
			t.User = defaultUser
		}
		out = append(out, t)
	}
	return out
}

// Dial opens an SSH client for the target.
func Dial(cfg Config, t HostTarget) (*ssh.Client, error) {
	user := t.User
	if user == "" {
		user = cfg.SSHUser
	}
	auth, err := authMethods(cfg)
	if err != nil {
		return nil, err
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("no SSH auth configured (LINUX_PATCH_SSH_KEY or LINUX_PATCH_SSH_PASSWORD)")
	}
	cc := &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // lab/ops; prefer known_hosts when set
		Timeout:         cfg.Timeout,
	}
	if cfg.Timeout <= 0 {
		cc.Timeout = 60 * time.Second
	}
	addr := net.JoinHostPort(t.Host, t.Port)
	return ssh.Dial("tcp", addr, cc)
}

// Run runs a remote command and returns combined output.
func Run(client *ssh.Client, cmd string) (string, error) {
	s, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	out, err := s.CombinedOutput(cmd)
	return string(out), err
}

func authMethods(cfg Config) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if cfg.SSHKeyPEM != "" {
		signer, err := ssh.ParsePrivateKey([]byte(cfg.SSHKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("parse SSH key PEM: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	} else if cfg.SSHKeyPath != "" {
		b, err := os.ReadFile(cfg.SSHKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read SSH key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(b)
		if err != nil {
			return nil, fmt.Errorf("parse SSH key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if cfg.SSHPassword != "" {
		methods = append(methods, ssh.Password(cfg.SSHPassword))
	}
	return methods, nil
}
