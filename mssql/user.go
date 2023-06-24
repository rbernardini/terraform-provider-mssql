package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/rbernardini/terraform-provider-mssql/model"
)

func (c *Connector) CreateUser(ctx context.Context, user *model.User) error {
	var version string
	err := c.QueryRowContext(ctx, "SELECT @@VERSION", func(row *sql.Row) error { return row.Scan(&version) })
	if err != nil {
		return err
	}

	stmtSQL := fmt.Sprintf("CREATE USER %s ", user.Username)
	if user.AuthType == "DATABASE" && user.LoginName == "" && user.Password == "" {
		return fmt.Errorf("for 'DATABASE' authentication type user password is required")
	}

	if user.LoginName != "" {
		stmtSQL += fmt.Sprintf("FOR LOGIN [%s]", user.LoginName)
	}
	if user.Password != "" {
		stmtSQL += fmt.Sprintf("WITH PASSWORD = '%s'", user.Password)
	}
	if user.AuthType == "EXTERNAL" {
		if strings.Contains(version, "Microsoft SQL Azure") {
			if user.ObjectId != "" {
				stmtSQL += " WITH SID=CONVERT(varchar(64), CAST(CAST(" + user.ObjectId +
					" AS UNIQUEIDENTIFIER) AS VARBINARY(16)), 1), TYPE=E"
			} else {
				stmtSQL += " FROM EXTERNAL PROVIDER"
			}
		}
	}

	if len(user.Options) > 0 {
		if !strings.Contains(stmtSQL, " WITH ") {
			stmtSQL += " WITH "
		}
		for opt := range user.Options {
			value := user.Options[opt].ValueOrSqlNull()
			stmtSQL += fmt.Sprintf(" %s = %s,", opt, value)
		}
		stmtSQL = strings.TrimRight(stmtSQL, ", ")
	}

	log.Printf("Using database: '%s'", user.Database)
	log.Printf("Executing statement: %s", stmtSQL)
	err = c.
		setDatabase(user.Database).
		ExecContext(ctx, stmtSQL)
	return err
}

func (c *Connector) DeleteUser(ctx context.Context, user *model.User) error {
	stmtSQL := fmt.Sprintf("IF EXISTS (SELECT 1 FROM [%s].[sys].[database_principals] WHERE [name] = '%s') "+
		"DROP USER %s", user.Database, user.Username, user.Username)

	log.Printf("Executing statement: %s", stmtSQL)

	return c.setDatabase(user.Database).
		ExecContext(ctx, stmtSQL)
}

func (c *Connector) GetUserRoles(ctx context.Context, username string) ([]string, error) {
	roles := make([]string, 0)
	err := c.QueryContext(ctx, `select r.name
		from
			master.sys.server_role_members rm
			inner join
			master.sys.server_principals r on r.principal_id = rm.role_principal_id and r.type = 'R'
			inner join
			master.sys.server_principals m on m.principal_id = rm.member_principal_id
		where m.name = @username`,
		func(rows *sql.Rows) error {
			var roleName string
			err := rows.Scan(&roleName)
			if err == nil {
				roles = append(roles, roleName)
			}
			return err
		}, sql.Named("username", username))

	return roles, err
}

func (c *Connector) GetUser(ctx context.Context, database string, username string) (*model.User, error) {
	stmtSQL := fmt.Sprintf(`SELECT 
		p.principal_id, p.name, p.authentication_type_desc, p.default_schema_name, p.default_language_name, p.sid
		FROM [%s].[sys].[database_principals] p 
		WHERE p.type = 'S' AND p.name LIKE '%s'`, database, username)
	log.Printf("Executing statement: %s", stmtSQL)
	var defaultSchema, defaultLanguage model.NullString
	var sid []byte
	user := &model.User{}
	err := c.QueryRowContext(ctx, stmtSQL, func(row *sql.Row) error {
		return row.Scan(&user.PrincipalID, &user.Username, &user.AuthType, &defaultSchema, &defaultLanguage, &sid)
	})
	if err != nil {
		return nil, err
	}
	user.Database = database
	user.Options = make(model.OptionsList)
	if defaultSchema != "" {
		user.Options["default_schema"] = defaultSchema
	}
	if defaultLanguage != "" {
		user.Options["default_language"] = defaultLanguage
	}

	return user, err
}

func ParseUserId(id string) (database string, username string, err error) {
	lastSeparatorIndex := strings.LastIndex(id, "/")

	if lastSeparatorIndex <= 0 {
		return "", "", fmt.Errorf("wrong ID format %s (expected database/username)", id)
	}

	database = id[0:lastSeparatorIndex]
	username = id[lastSeparatorIndex+1:]
	return
}
