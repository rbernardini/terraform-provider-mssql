package mssql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	mssql "github.com/denisenkom/go-mssqldb"
	"github.com/pkg/errors"
	"log"
	"net/url"
	"strings"
	"time"
)

type Connector struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Database   string `json:"database"`
	Login      *LoginUser
	AzureLogin *AzureLogin
	Timeout    time.Duration `json:"timeout,omitempty"`
	Token      string
}

type LoginUser struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type AzureLogin struct {
	TenantID     string `json:"tenant_id,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

func (c *Connector) setDatabase(database string) *Connector {
	c.Database = database
	if database == "" {
		c.Database = "master"
	}
	return c
}

func (c *Connector) PingContext(ctx context.Context) error {
	db, err := c.db()
	if err != nil {
		return err
	}

	err = db.PingContext(ctx)
	if err != nil {
		return errors.Wrap(err, "In ping")
	}

	return nil
}

// Execute an SQL statement and ignore the results
func (c *Connector) ExecContext(ctx context.Context, command string, args ...interface{}) error {
	db, err := c.db()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, command, args...)
	if err != nil {
		return err
	}

	return nil
}

func (c *Connector) QueryContext(ctx context.Context, query string, scanner func(*sql.Rows) error, args ...interface{}) error {
	db, err := c.db()
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	return scanner(rows)
}

func (c *Connector) QueryRowContext(ctx context.Context, query string, scanner func(*sql.Row) error, args ...interface{}) error {
	db, err := c.db()
	if err != nil {
		return err
	}
	defer db.Close()

	row := db.QueryRowContext(ctx, query, args...)
	if row.Err() != nil {
		return row.Err()
	}

	return scanner(row)
}

func (c *Connector) db() (*sql.DB, error) {
	if c == nil {
		panic("No connector")
	}
	conn, err := c.connector()
	if err != nil {
		return nil, err
	}
	if db, err := connectLoop(conn, c.Timeout); err != nil {
		return nil, err
	} else {
		return db, nil
	}
}

func (c *Connector) connector() (driver.Connector, error) {
	connectionString := c.ConnectionString()
	if c.Login != nil {
		return mssql.NewConnector(connectionString)
	}
	return mssql.NewAccessTokenConnector(connectionString, func() (string, error) { return c.tokenProvider() })
}

func (c *Connector) ConnectionString() string {
	query := url.Values{}
	if c.Database != "" {
		query.Set("database", c.Database)
	}
	return (&url.URL{
		Scheme:   "sqlserver",
		User:     c.userPassword(),
		Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		RawQuery: query.Encode(),
	}).String()
}

func (c *Connector) userPassword() *url.Userinfo {
	if c.Login != nil {
		return url.UserPassword(c.Login.Username, c.Login.Password)
	}
	return nil
}

func (c *Connector) tokenProvider() (string, error) {
	const resourceID = "https://database.windows.net/"

	admin := c.AzureLogin
	oauthConfig, err := adal.NewOAuthConfig(azure.PublicCloud.ActiveDirectoryEndpoint, admin.TenantID)
	if err != nil {
		return "", err
	}

	spt, err := adal.NewServicePrincipalToken(*oauthConfig, admin.ClientID, admin.ClientSecret, resourceID)
	if err != nil {
		return "", err
	}

	err = spt.EnsureFresh()
	if err != nil {
		return "", err
	}

	c.Token = spt.OAuthToken()

	return spt.OAuthToken(), nil
}

func connectLoop(connector driver.Connector, timeout time.Duration) (*sql.DB, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	timeoutExceeded := time.After(timeout)
	for {
		select {
		case <-timeoutExceeded:
			return nil, fmt.Errorf("db connection failed after %s timeout", timeout)

		case <-ticker.C:
			db, err := connect(connector)
			if err == nil {
				return db, nil
			}
			if strings.Contains(err.Error(), "Login failed") {
				return nil, err
			}
			if strings.Contains(err.Error(), "Login error") {
				return nil, err
			}
			if strings.Contains(err.Error(), "error retrieving access token") {
				return nil, err
			}
			log.Println(errors.Wrap(err, "failed to connect to database"))
		}
	}
}

func connect(connector driver.Connector) (*sql.DB, error) {
	db := sql.OpenDB(connector)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func quoteIdentifier(id string) string {
	return id
}
