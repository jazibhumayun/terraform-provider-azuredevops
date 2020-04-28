package branchpolicy

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/microsoft/azure-devops-go-api/azuredevops/policy"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/utils"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/utils/config"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/utils/converter"
)

// Policy type IDs. These are global and can be listed using the following endpoint:
//	https://docs.microsoft.com/en-us/rest/api/azure/devops/policy/types/list?view=azure-devops-rest-5.1
var (
	NoActiveComments = uuid.MustParse("c6a1889d-b943-4856-b76f-9e46bb6b0df2")
	MinReviewerCount = uuid.MustParse("fa4e907d-c16b-4a4c-9dfa-4906e5d171dd")
	SuccessfulBuild  = uuid.MustParse("0609b952-1397-4640-95ec-e00a01b2c241")
)

// Keys for schema elements
const (
	SchemaProjectID     = "project_id"
	SchemaEnabled       = "enabled"
	SchemaBlocking      = "blocking"
	SchemaSettings      = "settings"
	SchemaScope         = "scope"
	SchemaRepositoryID  = "repository_id"
	SchemaRepositoryRef = "repository_ref"
	SchemaMatchType     = "match_type"
)

// The type of repository branch name matching strategy used by the policy
const (
	matchTypeExact  string = "Exact"
	matchTypePrefix string = "Prefix"
)

// PolicyCrudArgs arguments for GenBasePolicyResource
type PolicyCrudArgs struct {
	FlattenFunc func(d *schema.ResourceData, policy *policy.PolicyConfiguration, projectID *string)
	ExpandFunc  func(d *schema.ResourceData, typeID uuid.UUID) (*policy.PolicyConfiguration, *string, error)
	PolicyType  uuid.UUID
}

type commonPolicySettings struct {
	Scopes []struct {
		RepositoryID      string `json:"repositoryId"`
		RepositoryRefName string `json:"refName"`
		MatchType         string `json:"matchKind"`
	} `json:"scope"`
}

// GenBasePolicyResource creates a Resource with the common elements of a build policy
func GenBasePolicyResource(crudArgs *PolicyCrudArgs) *schema.Resource {
	return &schema.Resource{
		Create:   genPolicyCreateFunc(crudArgs),
		Read:     genPolicyReadFunc(crudArgs),
		Update:   genPolicyUpdateFunc(crudArgs),
		Delete:   genPolicyDeleteFunc(crudArgs),
		Importer: genPolicyImporter(),
		Schema:   genBaseSchema(),
	}
}

// BaseFlattenFunc flattens each of the base elements of the schema
func BaseFlattenFunc(d *schema.ResourceData, policyConfig *policy.PolicyConfiguration, projectID *string) {
	d.SetId(strconv.Itoa(*policyConfig.Id))
	d.Set(SchemaProjectID, converter.ToString(projectID, ""))
	d.Set(SchemaEnabled, converter.ToBool(policyConfig.IsEnabled, true))
	d.Set(SchemaBlocking, converter.ToBool(policyConfig.IsBlocking, true))
	d.Set(SchemaSettings, flattenSettings(d, policyConfig))
}

func flattenSettings(d *schema.ResourceData, policyConfig *policy.PolicyConfiguration) []interface{} {
	policySettings := commonPolicySettings{}
	json.Unmarshal([]byte(fmt.Sprintf("%v", policyConfig.Settings)), &policySettings)

	scopes := make([]interface{}, len(policySettings.Scopes))
	for index, scope := range policySettings.Scopes {
		scopes[index] = map[string]interface{}{
			SchemaScope: map[string]interface{}{
				SchemaRepositoryID:  scope.RepositoryID,
				SchemaRepositoryRef: scope.RepositoryRefName,
				SchemaMatchType:     scope.MatchType,
			},
		}
	}
	return []interface{}{
		map[string]interface{}{
			SchemaScope: scopes,
		},
	}
}

// BaseExpandFunc expands each of the base elements of the schema
func BaseExpandFunc(d *schema.ResourceData, typeID uuid.UUID) (*policy.PolicyConfiguration, *string, error) {
	projectID := d.Get(SchemaProjectID).(string)

	policyConfig := policy.PolicyConfiguration{
		IsEnabled:  converter.Bool(d.Get(SchemaEnabled).(bool)),
		IsBlocking: converter.Bool(d.Get(SchemaBlocking).(bool)),
		Type: &policy.PolicyTypeRef{
			Id: &typeID,
		},
		Settings: expandSettings(d),
	}

	if d.Id() != "" {
		policyID, err := strconv.Atoi(d.Id())
		if err != nil {
			return nil, nil, fmt.Errorf("Error parsing policy configuration ID: (%+v)", err)
		}
		policyConfig.Id = &policyID
	}

	return &policyConfig, &projectID, nil
}

func expandSettings(d *schema.ResourceData) map[string]interface{} {
	settingsList := d.Get(SchemaSettings).([]interface{})
	settings := settingsList[0].(map[string]interface{})
	settingsScopes := settings[SchemaScope].([]interface{})

	scopes := make([]interface{}, len(settingsScopes))
	for index, scope := range settingsScopes {
		scopeMap := scope.(map[string]interface{})
		scopes[index] = map[string]interface{}{
			"repositoryId": scopeMap[SchemaRepositoryID],
			"refName":      scopeMap[SchemaRepositoryRef],
			"matchKind":    scopeMap[SchemaMatchType],
		}
	}
	return map[string]interface{}{
		SchemaScope: scopes,
	}
}

func genBaseSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		SchemaProjectID: {
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
		},
		SchemaEnabled: {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  true,
		},
		SchemaBlocking: {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  true,
		},
		SchemaSettings: {
			Type: schema.TypeList,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					SchemaScope: {
						Type: schema.TypeList,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								SchemaRepositoryID: {
									Type:     schema.TypeString,
									Optional: true,
								},
								SchemaRepositoryRef: {
									Type:     schema.TypeString,
									Optional: true,
								},
								SchemaMatchType: {
									Type:     schema.TypeString,
									Optional: true,
									Default:  matchTypeExact,
									ValidateFunc: validation.StringInSlice([]string{
										matchTypeExact, matchTypePrefix,
									}, true),
								},
							},
						},
						Required: true,
						MinItems: 1,
					},
				},
			},
			Required: true,
			MinItems: 1,
			MaxItems: 1,
		},
	}
}

func genPolicyCreateFunc(crudArgs *PolicyCrudArgs) schema.CreateFunc {
	return func(d *schema.ResourceData, m interface{}) error {
		clients := m.(*config.AggregatedClient)
		policyConfig, projectID, err := crudArgs.ExpandFunc(d, crudArgs.PolicyType)
		if err != nil {
			return err
		}

		createdPolicy, err := clients.PolicyClient.CreatePolicyConfiguration(clients.Ctx, policy.CreatePolicyConfigurationArgs{
			Configuration: policyConfig,
			Project:       projectID,
		})

		if err != nil) {
			return fmt.Errorf("Error creating policy in Azure DevOps: %+v", err)
		}

		crudArgs.FlattenFunc(d, createdPolicy, projectID)
		return nil
	}
}

func genPolicyReadFunc(crudArgs *PolicyCrudArgs) schema.ReadFunc {
	return func(d *schema.ResourceData, m interface{}) error {
		clients := m.(*config.AggregatedClient)
		projectID := d.Get(SchemaProjectID).(string)
		policyID, err := strconv.Atoi(d.Id())

		if err != nil {
			return fmt.Errorf("Error converting policy ID to an integer: (%+v)", err)
		}

		policyConfig, err := clients.PolicyClient.GetPolicyConfiguration(clients.Ctx, policy.GetPolicyConfigurationArgs{
			Project:         &projectID,
			ConfigurationId: &policyID,
		})

		if utils.ResponseWasNotFound(err) {
			d.SetId("")
			return nil
		}

		if err != nil {
			return fmt.Errorf("Error looking up build policy configuration with ID (%v) and project ID (%v): %v", policyID, projectID, err)
		}

		crudArgs.FlattenFunc(d, policyConfig, &projectID)
		return nil
	}
}

func genPolicyUpdateFunc(crudArgs *PolicyCrudArgs) schema.UpdateFunc {
	return func(d *schema.ResourceData, m interface{}) error {
		clients := m.(*config.AggregatedClient)
		policyConfig, projectID, err := crudArgs.ExpandFunc(d, crudArgs.PolicyType)
		if err != nil {
			return err
		}

		updatedPolicy, err := clients.PolicyClient.UpdatePolicyConfiguration(clients.Ctx, policy.UpdatePolicyConfigurationArgs{
			ConfigurationId: policyConfig.Id,
			Configuration:   policyConfig,
			Project:         projectID,
		})

		if err != nil {
			return fmt.Errorf("Error updating policy in Azure DevOps: %+v", err)
		}

		crudArgs.FlattenFunc(d, updatedPolicy, projectID)
		return nil
	}
}

func genPolicyDeleteFunc(crudArgs *PolicyCrudArgs) schema.DeleteFunc {
	return func(d *schema.ResourceData, m interface{}) error {
		clients := m.(*config.AggregatedClient)
		policyConfig, projectID, err := crudArgs.ExpandFunc(d, crudArgs.PolicyType)
		if err != nil {
			return err
		}

		err = clients.PolicyClient.DeletePolicyConfiguration(clients.Ctx, policy.DeletePolicyConfigurationArgs{
			ConfigurationId: policyConfig.Id,
			Project:         projectID,
		})

		if err != nil {
			return fmt.Errorf("Error deleting policy in Azure DevOps: %+v", err)
		}

		return nil
	}
}

func genPolicyImporter() *schema.ResourceImporter {
	return &schema.ResourceImporter{
		State: func(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
			id := d.Id()
			parts := strings.SplitN(id, "/", 2)
			if len(parts) != 2 || strings.EqualFold(parts[0], "") || strings.EqualFold(parts[1], "") {
				return nil, fmt.Errorf("unexpected format of ID (%s), expected projectid/resourceId", id)
			}

			_, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("Policy configuration ID (%s) isn't a valid Int", parts[1])
			}

			d.Set(SchemaProjectID, parts[0])
			d.SetId(parts[1])
			return []*schema.ResourceData{d}, nil
		},
	}
}
