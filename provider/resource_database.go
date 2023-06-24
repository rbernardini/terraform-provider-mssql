package provider

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/rbernardini/terraform-provider-mssql/model"
	"github.com/rbernardini/terraform-provider-mssql/mssql"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ResourceDatabase() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateDatabase,
		ReadContext:   ReadDatabase,
		UpdateContext: UpdateDatabase,
		DeleteContext: DeleteDatabase,
		Importer: &schema.ResourceImporter{
			StateContext: ImportDatabase,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"default_collation": {
				Type:     schema.TypeString,
				Optional: true,
				//Default:  "SQL_Latin1_General_CP1_CI_AS",
			},

			"options": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     schema.TypeString,
			},
		},
	}
}

func CreateDatabase(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	database := new(model.Database).Parse(data)

	stmtSQL := fmt.Sprintf("CREATE DATABASE [%s]", database.Name)
	if database.DefaultCollation != "" {
		stmtSQL += " COLLATE " + database.DefaultCollation
	}
	if len(database.Options) > 0 {
		stmtSQL += " WITH "
		for opt := range database.Options {
			stmtSQL += fmt.Sprintf(" %s = %s,", opt, database.Options[opt].ValueOrSqlNull())
		}
		stmtSQL = strings.TrimRight(stmtSQL, ",")
	}

	err := connector.ExecContext(ctx, stmtSQL)
	if err == nil {
		data.SetId(database.Name)
	}
	return diag.FromErr(err)
}

func ReadDatabase(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	database := new(model.Database).Parse(data)

	stmtSQL := "SELECT name, collation_name FROM sys.databases WHERE name LIKE '" + data.Id() + "'"

	log.Println("Executing statement:", stmtSQL)
	var collation model.NullString
	err := connector.QueryRowContext(ctx, stmtSQL, func(row *sql.Row) error {
		return row.Scan(&database.Name, &collation)
	})
	if err != nil {
		return diag.Diagnostics{diag.Diagnostic{
			Summary: fmt.Sprintf("read database %s info", data.Id()),
			Detail:  err.Error(),
		}}
	} else {
		_, ok := data.GetOk("default_collation")
		if ok {
			// If desired collation value is defined, then import actual collation value
			database.DefaultCollation = collation.ToString()
		}
	}

	return database.ToSchema(data)
}

func UpdateDatabase(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	diags := diag.Diagnostics{}

	database := new(model.Database).Parse(data)

	if data.HasChanges("default_collation") && database.DefaultCollation != "" {
		stmtSQL := fmt.Sprintf("ALTER DATABASE %s COLLATE %s", database.Name, database.DefaultCollation)
		//diags = append(diags, diag.Diagnostic{Severity: diag.Warning, Summary: stmtSQL})
		err := connector.ExecContext(ctx, stmtSQL)
		if err != nil {
			diags = diag.FromErr(err)
		}
	}

	if data.HasChange("options") {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "MSSQL database updates is not versatile",
			Detail:   "options updates may not detect and apply correctly, especially option remove",
		})
		//old, now := data.GetChange("options")
		//diags = append(diags, diag.Diagnostic{
		//	Severity: diag.Warning,
		//	Summary:  fmt.Sprintf("old options = %v", old),
		//	Detail:   fmt.Sprintf("new options = %v", now),
		//})

		for opt := range database.Options {
			value := database.Options[opt].ValueOrSqlNull()
			stmtSQL := fmt.Sprintf("ALTER DATABASE [%s] WITH %s = %s", database.Name, opt, value)
			log.Println("Executing statement:", stmtSQL)
			err := connector.ExecContext(ctx, stmtSQL)
			if err != nil {
				diags = append(diags, diag.Diagnostic{
					Summary: fmt.Sprintf("MSSQL database %s option '%s' update", database.Name, opt),
					Detail:  err.Error(),
				})
			}
		}
	}
	if !diags.HasError() {
		database.ToSchema(data)
	}

	return diags
}

func DeleteDatabase(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)

	stmtSQL := "DROP DATABASE " + data.Get("name").(string)
	log.Println("Executing statement:", stmtSQL)
	err := connector.ExecContext(ctx, stmtSQL)

	if err == nil {
		data.SetId("")
	}
	return diag.FromErr(err)
}

func ImportDatabase(ctx context.Context, data *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	diags := ReadDatabase(ctx, data, meta)

	if diags.HasError() {
		return nil, fmt.Errorf(diags[0].Summary)
	}

	return []*schema.ResourceData{data}, nil
}
