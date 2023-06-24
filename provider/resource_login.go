package provider

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/rbernardini/terraform-provider-mssql/model"
	"github.com/rbernardini/terraform-provider-mssql/mssql"
)

func ResourceLogin() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateLogin,
		ReadContext:   ReadLogin,
		UpdateContext: UpdateLogin,
		DeleteContext: DeleteLogin,

		Importer: &schema.ResourceImporter{
			StateContext: ImportLogin,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"password": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				StateFunc: func(src interface{}) string {
					return "" // Do not store password in state, actually
				},
			},
			"options": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     schema.TypeString,
			},
		},
	}
}

func CreateLogin(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	login := new(model.Login).Parse(data)
	stmtSQL := "CREATE LOGIN [" + login.Name + "]"
	if login.Password != "" || len(login.Options) > 0 {
		stmtSQL += " WITH "
		if login.Password != "" {
			stmtSQL += fmt.Sprintf(" PASSWORD = '%s', ", login.Password)
		}
		for opt := range login.Options {
			value := login.Options[opt].ValueOrSqlNull()
			log.Printf("option '%s' = '%s'", opt, value)
			stmtSQL += fmt.Sprintf(" %s = %s,", opt, value)
		}
		stmtSQL = strings.TrimRight(stmtSQL, ",")
	}

	log.Printf("Executing statement: %s", stmtSQL)
	err := connector.ExecContext(ctx, stmtSQL)
	if err == nil {
		data.SetId(login.Name)
	}

	return diag.FromErr(err)
}

func ReadLogin(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	login := new(model.Login).Parse(data)

	var defaultDatabase, defaultLanguage model.NullString
	stmtSQL := "SELECT name, default_database_name, default_language_name FROM [master].[sys].[sql_logins] WHERE [name] = @name"
	log.Printf("Executing statement: %s", stmtSQL)
	err := connector.QueryRowContext(ctx,
		stmtSQL,
		func(r *sql.Row) error {
			return r.Scan(&login.Name, &defaultDatabase, &defaultLanguage)
		},
		sql.Named("name", data.Id()),
	)
	if err != nil {
		return diag.FromErr(err)
	}

	log.Printf("READ: name='%s', password='%s'", login.Name, login.Password)

	log.Println("Importing Options")

	options := data.Get("options").(map[string]interface{})
	if options != nil {
		log.Printf("Options len= %d", len(options))
		_, ok := options["default_database"]
		if ok && defaultDatabase != "" {
			// If DEFAULT_DATABASE option defined explicitly, then import it, otherwise, skip
			login.Options["default_database"] = defaultDatabase
			log.Printf("Options: default_database = %s", defaultDatabase)
		}

		_, ok = options["options.default_language"]
		if ok && defaultLanguage != "" {
			// If DEFAULT_LANGUAGE option defined explicitly, then import it, otherwise, skip
			login.Options["default_language"] = defaultLanguage
			log.Printf("Options: default_language = %s", defaultLanguage)
		}
	} else {
		log.Println("No options")
	}

	return login.ToSchema(data)
}

func UpdateLogin(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	login := new(model.Login).Parse(data)
	diags := diag.Diagnostics{}

	if data.HasChange("options") {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "MSSQL login updates is not versatile",
			Detail:   "options updates may not detect and apply correctly, especially option remove",
		})
		//old, now := data.GetChange("options")
		//diags = append(diags, diag.Diagnostic{
		//	Severity: diag.Warning,
		//	Summary:  fmt.Sprintf("old options = %v", old),
		//	Detail:   fmt.Sprintf("new options = %v", now),
		//})

		for opt := range login.Options {
			value := login.Options[opt].ValueOrSqlNull()
			stmtSQL := fmt.Sprintf("ALTER LOGIN [%s] WITH %s = %s", login.Name, opt, value)
			log.Printf("Executing stagement: %s", stmtSQL)
			err := connector.ExecContext(ctx, stmtSQL)
			if err != nil {
				diags = append(diags, diag.Diagnostic{
					Summary: fmt.Sprintf("MSSQL login %s option '%s' update", login.Name, opt),
					Detail:  err.Error(),
				})
			}
		}
	}

	return diags
}

func DeleteLogin(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	name := data.Id()

	err := killSessionsForLogin(connector, ctx, name)
	if err != nil {
		return diag.FromErr(err)
	}

	stmtSQL := fmt.Sprintf("IF EXISTS (SELECT 1 FROM [master].[sys].[sql_logins] WHERE [name] = '%s') DROP LOGIN [%s]", name, name)
	err = connector.ExecContext(ctx, stmtSQL)
	if err == nil {
		data.SetId("")
	}

	return diag.FromErr(err)
}

func ImportLogin(ctx context.Context, data *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	diags := ReadLogin(ctx, data, meta)
	if diags.HasError() {
		return nil, fmt.Errorf(diags[0].Summary)
	}

	return []*schema.ResourceData{data}, nil
}

func killSessionsForLogin(c *mssql.Connector, ctx context.Context, name string) error {
	// adapted from https://stackoverflow.com/a/5178097/38055
	cmd := ` 
          DECLARE sessionsToKill CURSOR FAST_FORWARD FOR
            SELECT session_id
            FROM sys.dm_exec_sessions
            WHERE login_name = @name
          OPEN sessionsToKill
          DECLARE @sessionId INT
          DECLARE @statement NVARCHAR(200)
          FETCH NEXT FROM sessionsToKill INTO @sessionId
          WHILE @@FETCH_STATUS = 0
          BEGIN
            PRINT 'Killing session ' + CAST(@sessionId AS NVARCHAR(20)) + ' for model ' + @name
            SET @statement = 'KILL ' + CAST(@sessionId AS NVARCHAR(20))
            EXEC sp_executesql @statement
            FETCH NEXT FROM sessionsToKill INTO @sessionId
          END
          CLOSE sessionsToKill
          DEALLOCATE sessionsToKill`
	return c.ExecContext(ctx, cmd, sql.Named("name", name))
}
