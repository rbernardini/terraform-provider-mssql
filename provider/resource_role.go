package provider

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/rbernardini/terraform-provider-mssql/mssql"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ResourceRole() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateRole,
		ReadContext:   ReadRole,
		DeleteContext: DeleteRole,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func CreateRole(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	roleName := d.Get("name").(string)
	stmtSQL := fmt.Sprintf("CREATE ROLE '%s'", roleName)

	err := connector.ExecContext(ctx, stmtSQL)
	if err == nil {
		d.SetId(roleName)
	}
	return diag.FromErr(err)
}

func ReadRole(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)

	// SELECT name FROM [master].[sys].database_principals WHERE type = 'R'

	stmtSQL := fmt.Sprintf("SHOW GRANTS FOR '%s'", d.Id())

	err := connector.QueryContext(ctx, stmtSQL, func(rows *sql.Rows) error {
		return nil
	})
	if err != nil {
		log.Printf("[WARN] Role (%s) not found; removing from state", d.Id())
		d.SetId("")
		return diag.FromErr(err)
	}

	return diag.FromErr(d.Set("name", d.Id()))
}

func DeleteRole(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	stmtSQL := fmt.Sprintf("DROP ROLE '%s'", d.Get("name").(string))
	log.Printf("[DEBUG] SQL: %s", stmtSQL)
	err := connector.ExecContext(ctx, stmtSQL)

	return diag.FromErr(err)
}
