package db

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Validate checks if the database configuration is valid
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("database port must be between 1 and 65535, got %d", c.Port)
	}
	if c.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if c.Username == "" {
		return fmt.Errorf("database username is required")
	}
	if c.MaxOpenConns < 1 {
		return fmt.Errorf("max_open_conns must be at least 1")
	}
	if c.MaxIdleConns > c.MaxOpenConns {
		return fmt.Errorf("max_idle_conns cannot be greater than max_open_conns")
	}

	// Validate TLS configuration if SSL is enabled
	if c.SSL.Enabled && !c.SSL.SkipVerify {
		if err := c.validateTLSFiles(); err != nil {
			return fmt.Errorf("TLS configuration error: %w", err)
		}
	}

	return nil
}

// validateTLSFiles validates that TLS certificate files exist and are readable
func (c *Config) validateTLSFiles() error {
	// Validate CA file if provided
	if c.SSL.CAFile != "" {
		if _, err := os.Stat(c.SSL.CAFile); err != nil {
			return fmt.Errorf("CA file not accessible: %w", err)
		}
	}

	// Validate client certificate files if provided
	if c.SSL.CertFile != "" || c.SSL.KeyFile != "" {
		// Both cert and key must be provided together
		if c.SSL.CertFile == "" || c.SSL.KeyFile == "" {
			return fmt.Errorf("both CertFile and KeyFile must be provided together")
		}

		if _, err := os.Stat(c.SSL.CertFile); err != nil {
			return fmt.Errorf("client certificate file not accessible: %w", err)
		}

		if _, err := os.Stat(c.SSL.KeyFile); err != nil {
			return fmt.Errorf("client key file not accessible: %w", err)
		}
	}

	return nil
}

// GetDSN returns the MySQL Data Source Name using the official MySQL driver config builder
func (c *Config) GetDSN() string {
	// Use the official MySQL driver config builder for safe DSN construction
	cfg := mysql.Config{
		User:                 c.Username,
		Passwd:               c.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", c.Host, c.Port),
		DBName:               c.Database,
		Collation:            c.Collation,
		Loc:                  parseLocation(c.TimeZone),
		ParseTime:            true,
		AllowNativePasswords: true,
	}

	// Handle TLS configuration properly
	if c.SSL.Enabled {
		if c.SSL.SkipVerify {
			// Use skip-verify mode (not recommended for production)
			cfg.TLSConfig = "skip-verify"
		} else {
			// Build a TLS config and register it with the MySQL driver under a custom name
			tlsConfig := &tls.Config{
				InsecureSkipVerify: false,
			}

			// If a CA file is provided, load and validate it
			if c.SSL.CAFile != "" {
				caCert, err := os.ReadFile(c.SSL.CAFile)
				if err != nil {
					return fmt.Sprintf("mysql://%s@tcp(%s:%d)/%s?error=failed_to_read_ca_file",
						c.Username, c.Host, c.Port, c.Database)
				}
				pool := x509.NewCertPool()
				if !pool.AppendCertsFromPEM(caCert) {
					return fmt.Sprintf("mysql://%s@tcp(%s:%d)/%s?error=invalid_ca_certificate",
						c.Username, c.Host, c.Port, c.Database)
				}
				tlsConfig.RootCAs = pool
			}

			// If client cert/key provided, load them
			if c.SSL.CertFile != "" && c.SSL.KeyFile != "" {
				cert, err := tls.LoadX509KeyPair(c.SSL.CertFile, c.SSL.KeyFile)
				if err != nil {
					return fmt.Sprintf("mysql://%s@tcp(%s:%d)/%s?error=failed_to_load_client_cert",
						c.Username, c.Host, c.Port, c.Database)
				}
				tlsConfig.Certificates = []tls.Certificate{cert}
			}

			if c.SSL.ServerName != "" {
				tlsConfig.ServerName = c.SSL.ServerName
			}

			// Generate unique TLS config name based on config hash to prevent collisions
			// when multiple Config instances are used
			tlsName := c.generateTLSConfigName()
			if err := mysql.RegisterTLSConfig(tlsName, tlsConfig); err != nil {
				// If registration fails (e.g., name already exists with same config), that's OK
				// The driver will reuse the existing config
			}
			cfg.TLSConfig = tlsName
		}
	}

	return cfg.FormatDSN()
}

// generateTLSConfigName creates a unique name for TLS config registration
// based on the SSL configuration to prevent collisions between multiple Config instances
func (c *Config) generateTLSConfigName() string {
	// Create a hash of the SSL configuration to ensure uniqueness
	h := sha256.New()
	h.Write([]byte(c.SSL.CAFile))
	h.Write([]byte(c.SSL.CertFile))
	h.Write([]byte(c.SSL.KeyFile))
	h.Write([]byte(c.SSL.ServerName))
	hash := hex.EncodeToString(h.Sum(nil))[:16] // Use first 16 chars of hash
	return fmt.Sprintf("sql4go_tls_%s", hash)
}

// parseLocation parses timezone string to *time.Location
func parseLocation(tz string) *time.Location {
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		// Fallback to UTC if timezone parsing fails
		loc, _ = time.LoadLocation("UTC")
	}
	return loc
}
