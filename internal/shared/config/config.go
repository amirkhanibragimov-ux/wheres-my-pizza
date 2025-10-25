package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Database struct {
		Host     string
		Port     int
		User     string
		Password string
		Name     string // YAML key: "database"
	}
	RabbitMQ struct {
		Host     string
		Port     int
		User     string
		Password string
	}
}

// LoadFromFile loads config from a YAML file to a Config struct, applies defaults, and validates required fields.
func LoadFromFile(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var cfg Config
	if err := parseYAML(file, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	applyDefaults(&cfg)

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// applyDefaults sets safe defaults for some fields.
func applyDefaults(cfg *Config) {
	// Database
	if cfg.Database.Host == "" {
		cfg.Database.Host = "localhost"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}

	// RabbitMQ
	if cfg.RabbitMQ.Host == "" {
		cfg.RabbitMQ.Host = "localhost"
	}
	if cfg.RabbitMQ.Port == 0 {
		cfg.RabbitMQ.Port = 5672
	}
}

// validate checks required fields and basic ranges.
func (c *Config) validate() error {
	var problems []string

	// DB
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		problems = append(problems, "database.port must be in 1..65535")
	}
	if c.Database.User == "" {
		problems = append(problems, "database.user is required")
	}
	if c.Database.Password == "" {
		problems = append(problems, "database.password is required")
	}
	if c.Database.Name == "" {
		problems = append(problems, "database.database (name) is required")
	}

	// RabbitMQ
	if c.RabbitMQ.Port <= 0 || c.RabbitMQ.Port > 65535 {
		problems = append(problems, "rabbitmq.port must be in 1..65535")
	}
	if c.RabbitMQ.User == "" {
		problems = append(problems, "rabbitmq.user is required")
	}
	if c.RabbitMQ.Password == "" {
		problems = append(problems, "rabbitmq.password is required")
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// parseYAML parses the specific two-level mapping used by config.yaml.
func parseYAML(r io.Reader, cfg *Config) error {
	type section int
	const (
		none section = iota
		db
		rm
	)

	scanner := bufio.NewScanner(r)
	var cur section

	lineNo := 0
	seenTop := map[section]bool{}

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()

		// Strip comments
		if i := strings.IndexByte(raw, '#'); i >= 0 {
			raw = raw[:i]
		}

		line := strings.TrimRight(raw, " \t\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Top-level section? (no leading spaces)
		if len(line) > 0 && (line[0] != ' ' && line[0] != '\t') {
			switch strings.TrimSpace(line) {
			case "database:":
				cur = db
				if seenTop[db] {
					return fmt.Errorf("line %d: duplicate 'database' section", lineNo)
				}
				seenTop[db] = true
			case "rabbitmq:":
				cur = rm
				if seenTop[rm] {
					return fmt.Errorf("line %d: duplicate 'rabbitmq' section", lineNo)
				}
				seenTop[rm] = true
			default:
				return fmt.Errorf("line %d: unknown top-level key %q", lineNo, strings.TrimSuffix(strings.TrimSpace(line), ":"))
			}
			continue
		}

		// Expect indented "key: value"
		if cur == none {
			return fmt.Errorf("line %d: key without a section", lineNo)
		}
		trim := strings.TrimSpace(line)
		colon := strings.IndexByte(trim, ':')
		if colon <= 0 {
			return fmt.Errorf("line %d: expected 'key: value'", lineNo)
		}
		key := strings.TrimSpace(trim[:colon])
		val := strings.TrimSpace(trim[colon+1:])
		// Remove optional leading space in value
		val = strings.TrimLeft(val, " \t")

		switch cur {
		case db:
			switch key {
			case "host":
				cfg.Database.Host = val
			case "port":
				p, err := strconv.Atoi(val)
				if err != nil {
					return fmt.Errorf("line %d: database.port must be int: %v", lineNo, err)
				}
				cfg.Database.Port = p
			case "user":
				cfg.Database.User = val
			case "password":
				cfg.Database.Password = val
			case "database":
				cfg.Database.Name = val
			default:
				return fmt.Errorf("line %d: unknown key in database: %q", lineNo, key)
			}
		case rm:
			switch key {
			case "host":
				cfg.RabbitMQ.Host = val
			case "port":
				p, err := strconv.Atoi(val)
				if err != nil {
					return fmt.Errorf("line %d: rabbitmq.port must be int: %v", lineNo, err)
				}
				cfg.RabbitMQ.Port = p
			case "user":
				cfg.RabbitMQ.User = val
			case "password":
				cfg.RabbitMQ.Password = val
			default:
				return fmt.Errorf("line %d: unknown key in rabbitmq: %q", lineNo, key)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
