package terminal

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var connectionIDUnsafe = regexp.MustCompile(`[^a-z0-9._-]+`)

// ConnectionType identifies how a command should be executed.
type ConnectionType string

const (
	ConnectionTypeLocal ConnectionType = "local"
	ConnectionTypeSSH   ConnectionType = "ssh"
)

// Connection is the persisted FinalShell-style connection model used by both
// the business layer and GUI selectors. Passwords are intentionally kept out of
// logs and string renderings; callers should prefer key auth or an external
// secret store as the product matures.
type Connection struct {
	ID          string         `yaml:"id" json:"id"`
	Name        string         `yaml:"name" json:"name"`
	Type        ConnectionType `yaml:"type" json:"type"`
	Host        string         `yaml:"host,omitempty" json:"host,omitempty"`
	Port        int            `yaml:"port,omitempty" json:"port,omitempty"`
	Username    string         `yaml:"username,omitempty" json:"username,omitempty"`
	Password    string         `yaml:"password,omitempty" json:"password,omitempty"`
	PrivateKey  string         `yaml:"private_key,omitempty" json:"private_key,omitempty"`
	WorkingDir  string         `yaml:"working_dir,omitempty" json:"working_dir,omitempty"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
}

// DefaultLocalConnection returns a safe built-in local shell entry.
func DefaultLocalConnection() Connection {
	return Connection{
		ID:          "local-default",
		Name:        "Local Shell",
		Type:        ConnectionTypeLocal,
		Description: "Run commands on this machine",
	}
}

// Normalize fills backward-compatible defaults without hiding validation errors.
func (c *Connection) Normalize() {
	c.ID = strings.TrimSpace(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	c.Type = ConnectionType(strings.TrimSpace(string(c.Type)))
	c.Host = strings.TrimSpace(c.Host)
	c.Username = strings.TrimSpace(c.Username)
	c.WorkingDir = strings.TrimSpace(c.WorkingDir)
	if c.Type == "" {
		c.Type = ConnectionTypeLocal
	}
	if c.ID == "" {
		base := slugConnectionID(c.Name)
		base = strings.Trim(base, "-")
		if base == "" {
			base = string(c.Type)
		}
		c.ID = base
	}
	if c.Name == "" {
		switch c.Type {
		case ConnectionTypeLocal:
			c.Name = "Local Shell"
		case ConnectionTypeSSH:
			c.Name = c.Host
		}
	}
	if c.Type == ConnectionTypeSSH && c.Port == 0 {
		c.Port = 22
	}
}

// Validate checks whether the connection is executable.
func (c Connection) Validate() error {
	c.Normalize()
	if c.ID == "" {
		return errors.New("connection id is required")
	}
	if c.Name == "" {
		return errors.New("connection name is required")
	}
	switch c.Type {
	case ConnectionTypeLocal:
		return nil
	case ConnectionTypeSSH:
		if c.Host == "" {
			return errors.New("ssh host is required")
		}
		if c.Port < 1 || c.Port > 65535 {
			return fmt.Errorf("ssh port %d is out of range", c.Port)
		}
		if c.Username == "" {
			return errors.New("ssh username is required")
		}
		if c.Password == "" && c.PrivateKey == "" {
			return errors.New("ssh password or private key is required")
		}
		return nil
	default:
		return fmt.Errorf("unsupported connection type %q", c.Type)
	}
}

func NormalizeConnections(conns []Connection) []Connection {
	if len(conns) == 0 {
		return []Connection{DefaultLocalConnection()}
	}
	seen := make(map[string]struct{}, len(conns))
	out := make([]Connection, 0, len(conns))
	for _, conn := range conns {
		conn.Normalize()
		if conn.ID == "" {
			conn.ID = fmt.Sprintf("connection-%d", len(out)+1)
		}
		conn.ID = uniqueConnectionID(conn.ID, seen)
		out = append(out, conn)
	}
	return out
}

func slugConnectionID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = connectionIDUnsafe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func uniqueConnectionID(id string, seen map[string]struct{}) string {
	base := slugConnectionID(id)
	if base == "" {
		base = "connection"
	}
	candidate := base
	for i := 2; ; i++ {
		if _, ok := seen[candidate]; !ok {
			seen[candidate] = struct{}{}
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}
