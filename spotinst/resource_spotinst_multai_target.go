package spotinst

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/spotinst/spotinst-sdk-go/spotinst"
	"github.com/spotinst/spotinst-sdk-go/spotinst/util/stringutil"
)

func resourceSpotinstMultaiTarget() *schema.Resource {
	return &schema.Resource{
		Create: resourceSpotinstMultaiTargetCreate,
		Update: resourceSpotinstMultaiTargetUpdate,
		Read:   resourceSpotinstMultaiTargetRead,
		Delete: resourceSpotinstMultaiTargetDelete,

		Schema: map[string]*schema.Schema{
			"balancer_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"target_set_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"host": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"port": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
			},

			"weight": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
			},

			"tags": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
			},
		},
	}
}

func resourceSpotinstMultaiTargetCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*spotinst.Client)
	target, err := buildBalancerTargetOpts(d, meta)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] Target create configuration: %s",
		stringutil.Stringify(target))
	input := &spotinst.CreateTargetInput{
		Target: target,
	}
	resp, err := client.MultaiService.BalancerService().CreateTarget(context.Background(), input)
	if err != nil {
		return fmt.Errorf("Error creating target: %s", err)
	}
	d.SetId(spotinst.StringValue(resp.Target.ID))
	log.Printf("[INFO] Target created successfully: %s", d.Id())
	return resourceSpotinstMultaiTargetRead(d, meta)
}

func resourceSpotinstMultaiTargetRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*spotinst.Client)
	input := &spotinst.ReadTargetInput{
		TargetID: spotinst.String(d.Id()),
	}
	resp, err := client.MultaiService.BalancerService().ReadTarget(context.Background(), input)
	if err != nil {
		return fmt.Errorf("Error retrieving target: %s", err)
	}
	if t := resp.Target; t != nil {
		d.Set("balancer_id", t.BalancerID)
		d.Set("target_set_id", t.TargetSetID)
		d.Set("name", t.Name)
		d.Set("host", t.Host)
		d.Set("port", t.Port)
		d.Set("weight", t.Weight)
		d.Set("tags", flattenTags(t.Tags))
	} else {
		d.SetId("")
	}
	return nil
}

func resourceSpotinstMultaiTargetUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*spotinst.Client)
	target := &spotinst.Target{ID: spotinst.String(d.Id())}
	update := false

	if d.HasChange("name") {
		target.Name = spotinst.String(d.Get("name").(string))
		update = true
	}

	if d.HasChange("host") {
		target.Host = spotinst.String(d.Get("host").(string))
		update = true
	}

	if d.HasChange("port") {
		target.Port = spotinst.Int(d.Get("port").(int))
		update = true
	}

	if d.HasChange("weight") {
		target.Weight = spotinst.Int(d.Get("weight").(int))
		update = true
	}

	if d.HasChange("tags") {
		if v, ok := d.GetOk("tags"); ok {
			if tags, err := expandTags(v); err != nil {
				return err
			} else {
				target.Tags = tags
				update = true
			}
		}
	}

	if update {
		log.Printf("[DEBUG] Target update configuration: %s",
			stringutil.Stringify(target))
		input := &spotinst.UpdateTargetInput{
			Target: target,
		}
		if _, err := client.MultaiService.BalancerService().UpdateTarget(context.Background(), input); err != nil {
			return fmt.Errorf("Error updating target %s: %s", d.Id(), err)
		}
	}

	return resourceSpotinstMultaiTargetRead(d, meta)
}

func resourceSpotinstMultaiTargetDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*spotinst.Client)
	log.Printf("[INFO] Deleting target: %s", d.Id())
	input := &spotinst.DeleteTargetInput{
		TargetID: spotinst.String(d.Id()),
	}
	if _, err := client.MultaiService.BalancerService().DeleteTarget(context.Background(), input); err != nil {
		return fmt.Errorf("Error deleting target: %s", err)
	}
	d.SetId("")
	return nil
}

func buildBalancerTargetOpts(d *schema.ResourceData, meta interface{}) (*spotinst.Target, error) {
	target := &spotinst.Target{
		BalancerID:  spotinst.String(d.Get("balancer_id").(string)),
		TargetSetID: spotinst.String(d.Get("target_set_id").(string)),
		Name:        spotinst.String(d.Get("name").(string)),
		Host:        spotinst.String(d.Get("host").(string)),
		Port:        spotinst.Int(d.Get("port").(int)),
		Weight:      spotinst.Int(d.Get("weight").(int)),
	}
	if v, ok := d.GetOk("tags"); ok {
		if tags, err := expandTags(v); err != nil {
			return nil, err
		} else {
			target.Tags = tags
		}
	}
	return target, nil
}
