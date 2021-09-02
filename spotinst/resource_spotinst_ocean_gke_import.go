package spotinst

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/spotinst/spotinst-sdk-go/service/ocean/providers/gcp"
	"github.com/spotinst/spotinst-sdk-go/spotinst"
	"github.com/spotinst/spotinst-sdk-go/spotinst/client"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/commons"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import_autoscaler"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import_launch_specification"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import_scheduling"
)

func resourceSpotinstOceanGKEImport() *schema.Resource {
	setupClusterGKEImportResource()

	return &schema.Resource{
		Create: resourceSpotinstClusterGKEImportCreate,
		Read:   resourceSpotinstClusterGKEImportRead,
		Update: resourceSpotinstClusterGKEImportUpdate,
		Delete: resourceSpotinstClusterGKEImportDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: commons.OceanGKEImportResource.GetSchemaMap(),
	}
}

func setupClusterGKEImportResource() {
	fieldsMap := make(map[commons.FieldName]*commons.GenericField)

	ocean_gke_import.Setup(fieldsMap)
	ocean_gke_import_scheduling.Setup(fieldsMap)
	ocean_gke_import_autoscaler.Setup(fieldsMap)
	ocean_gke_import_launch_specification.Setup(fieldsMap)

	commons.OceanGKEImportResource = commons.NewOceanGKEImportResource(fieldsMap)
}

func importOceanGKECluster(resourceData *schema.ResourceData, meta interface{}) (*gcp.Cluster, error) {
	input := &gcp.ImportOceanGKEClusterInput{
		ClusterName: spotinst.String(resourceData.Get("cluster_name").(string)),
		Location:    spotinst.String(resourceData.Get("location").(string)),
	}

	resp, err := meta.(*Client).ocean.CloudProviderGCP().ImportOceanGKECluster(context.Background(), input)

	if err != nil {
		// If the group was not found, return nil so that we can show
		// that the group is gone.
		if errs, ok := err.(client.Errors); ok && len(errs) > 0 {
			for _, err := range errs {
				if err.Code == ErrCodeClusterNotFound {
					resourceData.SetId("")
					return nil, err
				}
			}
		}
		// Some other error, report it.
		return nil, fmt.Errorf("ocean GKE: import failed to read group: %s", err)
	}

	return resp.Cluster, err
}

func resourceSpotinstClusterGKEImportCreate(resourceData *schema.ResourceData, meta interface{}) error {
	log.Printf(string(commons.ResourceOnCreate),
		commons.OceanGKEImportResource.GetName())

	importedCluster, err := importOceanGKECluster(resourceData, meta.(*Client))
	if err != nil {
		return err
	}

	cluster, err := commons.OceanGKEImportResource.OnCreate(importedCluster, resourceData, meta)
	if err != nil {
		return err
	}

	clusterID, err := createGKEImportedCluster(cluster, meta.(*Client))
	if err != nil {
		return err
	}

	resourceData.SetId(spotinst.StringValue(clusterID))

	log.Printf("===> GKE imported cluster created successfully: %s <===", resourceData.Id())
	return resourceSpotinstClusterGKEImportRead(resourceData, meta)
}

func createGKEImportedCluster(cluster *gcp.Cluster, spotinstClient *Client) (*string, error) {
	if json, err := commons.ToJson(cluster); err != nil {
		return nil, err
	} else {
		log.Printf("===> Cluster GKE import configuration: %s", json)
	}

	var resp *gcp.CreateClusterOutput = nil
	err := resource.Retry(time.Minute, func() *resource.RetryError {
		input := &gcp.CreateClusterInput{Cluster: cluster}
		r, err := spotinstClient.ocean.CloudProviderGCP().CreateCluster(context.Background(), input)
		if err != nil {

			// Some other error, report it.
			return resource.NonRetryableError(err)
		}
		resp = r
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to create GKE imported cluster: %s", err)
	}
	return resp.Cluster.ID, nil
}

func resourceSpotinstClusterGKEImportRead(resourceData *schema.ResourceData, meta interface{}) error {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceOnRead),
		commons.OceanGKEImportResource.GetName(), id)

	input := &gcp.ReadClusterInput{ClusterID: spotinst.String(id)}
	resp, err := meta.(*Client).ocean.CloudProviderGCP().ReadCluster(context.Background(), input)

	if err != nil {
		// If the cluster was not found, return nil so that we can show
		// that the group does not exist
		if errs, ok := err.(client.Errors); ok && len(errs) > 0 {
			for _, err := range errs {
				if err.Code == ErrCodeClusterNotFound {
					resourceData.SetId("")
					return nil
				}
			}
		}

		// Some other error, report it.
		return fmt.Errorf("failed to read GKE cluster: %s", err)
	}

	// if nothing was found, return no state
	clusterResponse := resp.Cluster
	if clusterResponse == nil {
		resourceData.SetId("")
		return nil
	}

	// Expose the controller cluster identifier.
	if clusterResponse.ControllerClusterID != nil {
		for _, key := range []commons.FieldName{
			ocean_gke_import.ControllerClusterID,
			ocean_gke_import.ClusterControllerID, // maintains backward compatibility
		} {
			if err := resourceData.Set(string(key), *clusterResponse.ControllerClusterID); err != nil {
				log.Printf("[ERROR] Failed to set %q", string(key))
			}
		}
	}

	if err := commons.OceanGKEImportResource.OnRead(clusterResponse, resourceData, meta); err != nil {
		return err
	}
	log.Printf("===> GKE cluster read successfully: %s <===", id)
	return nil
}

func resourceSpotinstClusterGKEImportUpdate(resourceData *schema.ResourceData, meta interface{}) error {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceOnUpdate),
		commons.OceanGKEImportResource.GetName(), id)

	shouldUpdate, cluster, err := commons.OceanGKEImportResource.OnUpdate(resourceData, meta)
	if err != nil {
		return err
	}

	if shouldUpdate {
		cluster.SetId(spotinst.String(id))
		if err := updateGKEImportCluster(cluster, resourceData, meta); err != nil {
			return err
		}
	}
	log.Printf("===> GLE Cluster updated successfully: %s <===", id)
	return resourceSpotinstClusterGKEImportRead(resourceData, meta)
}

func updateGKEImportCluster(cluster *gcp.Cluster, resourceData *schema.ResourceData, meta interface{}) error {
	var input = &gcp.UpdateClusterInput{
		Cluster: cluster,
	}

	var shouldRoll = false
	clusterID := resourceData.Id()
	if updatePolicy, exists := resourceData.GetOkExists(string(ocean_gke_import.UpdatePolicy)); exists {
		list := updatePolicy.([]interface{})
		if len(list) > 0 && list[0] != nil {
			m := list[0].(map[string]interface{})

			if roll, ok := m[string(ocean_gke_import.ShouldRoll)].(bool); ok && roll {
				shouldRoll = roll
			}
		}
	}

	if json, err := commons.ToJson(cluster); err != nil {
		return err
	} else {
		log.Printf("===> GKE Cluster update configuration: %s", json)
	}

	if _, err := meta.(*Client).ocean.CloudProviderGCP().UpdateCluster(context.Background(), input); err != nil {
		return fmt.Errorf("[ERROR] Failed to update GKE cluster [%v]: %v", clusterID, err)
	} else if shouldRoll {
		if err := rollOceanGkeCluster(resourceData, meta); err != nil {
			log.Printf("[ERROR] Cluster [%v] roll failed, error: %v", clusterID, err)
			return err
		}
	} else {
		log.Printf("onRoll() -> Field [%v] is false, skipping cluster roll", string(ocean_gke_import.ShouldRoll))
	}

	return nil
}

func resourceSpotinstClusterGKEImportDelete(resourceData *schema.ResourceData, meta interface{}) error {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceOnDelete),
		commons.OceanGKEImportResource.GetName(), id)

	if err := deleteGKEImportCluster(resourceData, meta); err != nil {
		return err
	}

	log.Printf("===> GKE Cluster deleted successfully: %s <===", resourceData.Id())
	resourceData.SetId("")
	return nil
}

func deleteGKEImportCluster(resourceData *schema.ResourceData, meta interface{}) error {
	clusterID := resourceData.Id()
	input := &gcp.DeleteClusterInput{
		ClusterID: spotinst.String(clusterID),
	}

	if json, err := commons.ToJson(input); err != nil {
		return err
	} else {
		log.Printf("===> GKE Cluster delete configuration: %s", json)
	}

	if _, err := meta.(*Client).ocean.CloudProviderGCP().DeleteCluster(context.Background(), input); err != nil {
		return fmt.Errorf("[ERROR] onDelete() -> Failed to delete GKE cluster: %s", err)
	}
	return nil
}

func rollOceanGkeCluster(resourceData *schema.ResourceData, meta interface{}) error {
	var errResult error = nil
	clusterID := resourceData.Id()

	if updatePolicy, exists := resourceData.GetOkExists(string(ocean_gke_import.UpdatePolicy)); exists {
		list := updatePolicy.([]interface{})
		if len(list) > 0 && list[0] != nil {
			updateClusterSchema := list[0].(map[string]interface{})
			if rollConfig, ok := updateClusterSchema[string(ocean_gke_import.RollConfig)]; !ok || rollConfig == nil {
				errResult = fmt.Errorf("[ERROR] onRoll() -> Field [%v] is missing, skipping roll for cluster [%v]", string(ocean_gke_import.RollConfig), clusterID)
			} else {
				if rollClusterInput, err := expandOceanGKERollConfig(rollConfig, spotinst.String(clusterID)); err != nil {
					errResult = fmt.Errorf("[ERROR] onRoll() -> Failed expanding roll configuration for cluster [%v], error: %v", clusterID, err)
				} else {
					if json, err := commons.ToJson(rollConfig); err != nil {
						return err
					} else {
						log.Printf("onRoll() -> Rolling cluster [%v] with configuration %s", clusterID, json)
						rollClusterInput.Roll.ClusterID = spotinst.String(clusterID)
						_, err := meta.(*Client).ocean.CloudProviderGCP().CreateRoll(context.Background(), rollClusterInput)
						if err != nil {
							return fmt.Errorf("onRoll() -> Roll failed for cluster [%v], error: %v", clusterID, err)
						} else {
							log.Printf("onRoll() -> Successfully rolled cluster [%v]", clusterID)
						}
					}
				}
			}
		}
	} else {
		errResult = fmt.Errorf("[ERROR] onRoll() -> Missing update policy for cluster [%v]", clusterID)
	}

	if errResult != nil {
		return errResult
	}
	return nil
}

func expandOceanGKERollConfig(data interface{}, clusterID *string) (*gcp.CreateRollInput, error) {
	i := &gcp.CreateRollInput{Roll: &gcp.RollSpec{ClusterID: clusterID}}
	list := data.([]interface{})
	if list != nil && list[0] != nil {
		m := list[0].(map[string]interface{})

		if v, ok := m[string(ocean_gke_import.BatchSizePercentage)].(int); ok {
			i.Roll.BatchSizePercentage = spotinst.Int(v)
		}

		if v, ok := m[string(ocean_gke_import.LaunchSpecIDs)].([]string); ok {
			i.Roll.LaunchSpecIDs = expandOceanGKELaunchSpecIDs(v)
		}

	}
	return i, nil
}

func expandOceanGKELaunchSpecIDs(data interface{}) []string {
	list := data.([]interface{})
	result := make([]string, 0, len(list))

	for _, v := range list {
		if ls, ok := v.(string); ok && ls != "" {
			result = append(result, ls)
		}
	}
	return result
}
