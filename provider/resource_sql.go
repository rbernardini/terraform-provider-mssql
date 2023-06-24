package provider

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/rbernardini/terraform-provider-mssql/mssql"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ResourceSql() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateSql,
		ReadContext:   ReadSql,
		DeleteContext: DeleteSql,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"create_sql": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"delete_sql": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func CreateSql(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	name := d.Get("name").(string)
	createSql := d.Get("create_sql").(string)

	log.Println("Executing SQL", createSql)

	err := connector.ExecContext(ctx, createSql)

	if err == nil {
		d.SetId(name)
	}

	return diag.FromErr(err)
}

func ReadSql(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func DeleteSql(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	deleteSql := d.Get("delete_sql").(string)

	log.Println("Executing SQL:", deleteSql)

	err := connector.ExecContext(ctx, deleteSql)

	if err == nil {
		d.SetId("")
	}

	return diag.FromErr(err)
}
