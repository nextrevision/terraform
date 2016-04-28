package dockercloud

import (
	"fmt"
	"strings"
	"time"

	"github.com/docker/go-dockercloud/dockercloud"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceDockercloudNodeCluster() *schema.Resource {
	return &schema.Resource{
		Create: resourceDockercloudNodeClusterCreate,
		Read:   resourceDockercloudNodeClusterRead,
		Update: resourceDockercloudNodeClusterUpdate,
		Delete: resourceDockercloudNodeClusterDelete,
		Exists: resourceDockercloudNodeClusterExists,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"node_provider": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"size": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"region": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"disk": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"node_count": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				ForceNew: false,
			},
			"state": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"tags": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: false,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"provider_options": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				ForceNew: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"vpc": &schema.Schema{
							Type:     schema.TypeSet,
							Optional: true,
							ForceNew: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"id": &schema.Schema{
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},
									"subnets": &schema.Schema{
										Type:     schema.TypeList,
										Optional: true,
										ForceNew: true,
										Elem:     &schema.Schema{Type: schema.TypeString},
									},
									"security_groups": &schema.Schema{
										Type:     schema.TypeList,
										Optional: true,
										ForceNew: true,
										Elem:     &schema.Schema{Type: schema.TypeString},
									},
								},
							},
						},
						"iam": &schema.Schema{
							Type:     schema.TypeSet,
							Optional: true,
							ForceNew: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"instance_profile_name": &schema.Schema{
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func resourceDockercloudNodeClusterCreate(d *schema.ResourceData, meta interface{}) error {
	provider := d.Get("node_provider").(string)
	region := d.Get("region").(string)
	size := d.Get("size").(string)

	opts := &dockercloud.NodeCreateRequest{
		Name:     d.Get("name").(string),
		Region:   fmt.Sprintf("/api/infra/v1/region/%s/%s/", provider, region),
		NodeType: fmt.Sprintf("/api/infra/v1/nodetype/%s/%s/", provider, size),
	}

	if attr, ok := d.GetOk("disk"); ok {
		opts.Disk = attr.(int)
	}

	if attr, ok := d.GetOk("node_count"); ok {
		opts.Target_num_nodes = attr.(int)
	}

	tags := d.Get("tags.#").(int)
	if tags > 0 {
		opts.Tags = make([]dockercloud.NodeTag, 0, tags)
		for i := 0; i < tags; i++ {
			key := fmt.Sprintf("tags.%d", i)
			opts.Tags = append(opts.Tags, dockercloud.NodeTag{Name: d.Get(key).(string)})
		}
	}

	if attr, ok := d.GetOk("provider_options"); ok {
		providerOptions := attr.(*schema.Set).List()[0].(map[string]interface{})
		opts.Provider_options = &dockercloud.ProviderOption{
			Vpc: vpcSetToVPC(providerOptions["vpc"].(*schema.Set)),
			Iam: iamSetToIAM(providerOptions["iam"].(*schema.Set)),
		}
		fmt.Printf("%+v\n", opts.Provider_options)
	}

	nodeCluster, err := dockercloud.CreateNodeCluster(*opts)
	if err != nil {
		return err
	}

	if err = nodeCluster.Deploy(); err != nil {
		return fmt.Errorf("Error creating node cluster: %s", err)
	}

	d.SetId(nodeCluster.Uuid)
	d.Set("state", nodeCluster.State)

	stateConf := &resource.StateChangeConf{
		Pending:        []string{"Deploying"},
		Target:         []string{"Deployed"},
		Refresh:        newNodeClusterStateRefreshFunc(d, meta),
		Timeout:        60 * time.Minute,
		Delay:          10 * time.Second,
		MinTimeout:     3 * time.Second,
		NotFoundChecks: 60,
	}

	nodeClusterRaw, err := stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for node cluster (%s) to become ready: %s", d.Id(), err)
	}

	nodeCluster = nodeClusterRaw.(dockercloud.NodeCluster)
	d.Set("state", nodeCluster.State)

	return resourceDockercloudNodeClusterRead(d, meta)
}

func resourceDockercloudNodeClusterRead(d *schema.ResourceData, meta interface{}) error {
	nodeCluster, err := dockercloud.GetNodeCluster(d.Id())
	if err != nil {
		if strings.Contains(err.Error(), "404 NOT FOUND") {
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving node cluster: %s", err)
	}

	if nodeCluster.State == "Terminated" {
		d.SetId("")
		return nil
	}

	d.Set("name", nodeCluster.Name)
	d.Set("node_count", nodeCluster.Target_num_nodes)
	d.Set("disk", nodeCluster.Disk)
	d.Set("state", nodeCluster.State)

	return nil
}

func resourceDockercloudNodeClusterUpdate(d *schema.ResourceData, meta interface{}) error {
	opts := &dockercloud.NodeCreateRequest{}

	if d.HasChange("node_count") {
		_, newNum := d.GetChange("node_count")
		opts.Target_num_nodes = newNum.(int)
	}

	if d.HasChange("tags") {
		_, newTags := d.GetChange("tags")
		tags := newTags.([]interface{})
		opts.Tags = make([]dockercloud.NodeTag, 0, len(tags))

		for _, tag := range tags {
			opts.Tags = append(opts.Tags, dockercloud.NodeTag{Name: tag.(string)})
		}
	}

	nodeCluster, err := dockercloud.GetNodeCluster(d.Id())
	if err != nil {
		return fmt.Errorf("Error retrieving node cluster (%s): %s", d.Id(), err)
	}

	if err := nodeCluster.Update(*opts); err != nil {
		return fmt.Errorf("Error updating node cluster: %s", err)
	}

	stateConf := &resource.StateChangeConf{
		Pending:        []string{"Scaling"},
		Target:         []string{"Deployed"},
		Refresh:        newNodeClusterStateRefreshFunc(d, meta),
		Timeout:        60 * time.Minute,
		Delay:          10 * time.Second,
		MinTimeout:     3 * time.Second,
		NotFoundChecks: 60,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for node cluster (%s) to finish scaling: %s", d.Id(), err)
	}

	return nil
}

func resourceDockercloudNodeClusterDelete(d *schema.ResourceData, meta interface{}) error {
	nodeCluster, err := dockercloud.GetNodeCluster(d.Id())
	if err != nil {
		return fmt.Errorf("Error retrieving node cluster (%s): %s", d.Id(), err)
	}

	if nodeCluster.State == "Terminated" {
		d.SetId("")
		return nil
	}

	if err = nodeCluster.Terminate(); err != nil {
		return fmt.Errorf("Error deleting node cluster (%s): %s", d.Id(), err)
	}

	stateConf := &resource.StateChangeConf{
		Pending:        []string{"Terminating", "Empty cluster"},
		Target:         []string{"Terminated"},
		Refresh:        newNodeClusterStateRefreshFunc(d, meta),
		Timeout:        60 * time.Minute,
		Delay:          10 * time.Second,
		MinTimeout:     3 * time.Second,
		NotFoundChecks: 60,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for node cluster (%s) to terminate: %s", d.Id(), err)
	}

	d.SetId("")

	return nil
}

func resourceDockercloudNodeClusterExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	nodeCluster, err := dockercloud.GetNodeCluster(d.Id())
	if err != nil {
		return false, err
	}

	if nodeCluster.Uuid == d.Id() {
		return true, nil
	}

	return false, nil
}

func newNodeClusterStateRefreshFunc(d *schema.ResourceData, meta interface{}) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		nodeCluster, err := dockercloud.GetNodeCluster(d.Id())
		if err != nil {
			return nil, "", err
		}

		return nodeCluster, nodeCluster.State, nil
	}
}

func vpcSetToVPC(s *schema.Set) dockercloud.VPC {
	vpc := dockercloud.VPC{}
	subnets := []string{}
	security_groups := []string{}

	if s.Len() > 0 {
		vpcOptions := s.List()[0].(map[string]interface{})

		for _, subnet := range vpcOptions["subnets"].([]interface{}) {
			subnets = append(subnets, subnet.(string))
		}

		for _, sg := range vpcOptions["security_groups"].([]interface{}) {
			security_groups = append(security_groups, sg.(string))
		}

		vpc.Id = vpcOptions["id"].(string)
		vpc.Subnets = subnets
		vpc.Security_groups = security_groups
	}

	return vpc
}

func iamSetToIAM(s *schema.Set) dockercloud.IAM {
	iam := dockercloud.IAM{}

	if s.Len() > 0 {
		iamOptions := s.List()[0].(map[string]interface{})
		iam.Instance_profile_name = iamOptions["instance_profile_name"].(string)
	}

	return iam
}
