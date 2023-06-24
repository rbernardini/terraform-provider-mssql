package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/rbernardini/terraform-provider-mssql/model"
	"github.com/rbernardini/terraform-provider-mssql/mssql"
	"github.com/thoas/go-funk"
)

func ResourceUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateUser,
		UpdateContext: UpdateUser,
		ReadContext:   ReadUser,
		DeleteContext: DeleteUser,
		Importer: &schema.ResourceImporter{
			StateContext: ImportUser,
		},

		Schema: map[string]*schema.Schema{
			"database": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "In which database this user will be created",
			},
			"username": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"password": {
				Type:          schema.TypeString,
				Optional:      true,
				Sensitive:     true,
				Description:   "User password",
				ConflictsWith: []string{"login_name"},
			},
			"login_name": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				Description:   "Create user for existing [login] from 'master' database or Windows login name",
				ConflictsWith: []string{"object_id", "principal_id"},
			},

			"object_id": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				Description:   "External object ID",
				ConflictsWith: []string{"login_name", "principal_id"},
			},
			"principal_id": {
				Type:          schema.TypeInt,
				Optional:      true,
				ForceNew:      true,
				Description:   "Create user for existing [Principal ID] from 'master' database",
				ConflictsWith: []string{"object_id", "login_name"},
			},
			"auth_type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "DATABASE",
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					allowedValues := []string{"DATABASE", "INSTANCE", "EXTERNAL"}
					if !funk.ContainsString(allowedValues, val.(string)) {
						errs = append(errs, fmt.Errorf("auth_type must be one of: %s", allowedValues))
					}
					return
				},
			},
			"options": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     schema.TypeString,
			},
			"roles": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func CreateUser(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	user := new(model.User).Parse(data)

	err := connector.CreateUser(ctx, user)
	if err != nil {
		return diag.FromErr(err)
	}

	userId := fmt.Sprintf("%s/%s", user.Database, user.Username)
	data.SetId(userId)

	return nil
}

func UpdateUser(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	if err := connector.PingContext(ctx); err != nil {
		return diag.FromErr(err)
	}
	user := new(model.User).Parse(data)

	err := connector.UpdateUser(ctx, connector.Database, user)
	return diag.FromErr(err)
}

func ReadUser(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	database, username, err := mssql.ParseUserId(data.Id())

	user, err := connector.GetUser(ctx, database, username)
	diags := diag.FromErr(err)

	if user != nil {
		data.SetId(fmt.Sprintf("%s/%s", user.Database, user.Username))
		diags = append(diags, user.ToSchema(data)...)
	}
	return diags
}

func DeleteUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	connector := meta.(*mssql.Connector)
	user := new(model.User).Parse(d)

	err := connector.DeleteUser(ctx, user)
	if err == nil {
		d.SetId("")
	}
	return diag.FromErr(err)
}

func ImportUser(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	database, username, err := mssql.ParseUserId(d.Id())
	if err != nil {
		return nil, err
	}

	connector := meta.(*mssql.Connector)
	user, err := connector.GetUser(ctx, database, username)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, fmt.Errorf("user '%s' not found", d.Id())
	}

	user.ToSchema(d)

	return []*schema.ResourceData{d}, nil
}
