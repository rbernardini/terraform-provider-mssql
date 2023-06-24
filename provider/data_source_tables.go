package provider

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/rbernardini/terraform-provider-mssql/mssql"
)

func DataSourceTables() *schema.Resource {
	return &schema.Resource{
		ReadContext: ShowTables,
		Schema: map[string]*schema.Schema{
			"database": {
				Type:     schema.TypeString,
				Required: true,
			},
			"pattern": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"tables": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func ShowTables(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)

	database := d.Get("database").(string)
	pattern := d.Get("pattern").(string)

	stmtSQL := fmt.Sprintf("SELECT TABLE_NAME FROM %s.INFORMATION_SCHEMA.TABLES t WHERE TABLE_TYPE = 'BASE TABLE'", database)

	if pattern != "" {
		stmtSQL += fmt.Sprintf(" AND TABLE_NAME LIKE '%s'", pattern)
	}

	var tables []string

	err := connector.QueryContext(ctx, stmtSQL, func(rows *sql.Rows) error {
		for rows.Next() {
			var table string
			err := rows.Scan(&table)
			if err == nil {
				tables = append(tables, table)
			}
			return err
		}
		return rows.Close()
	})

	err = d.Set("tables", tables)

	if err == nil {
		d.SetId(resource.UniqueId())
	}

	return diag.FromErr(err)
}
