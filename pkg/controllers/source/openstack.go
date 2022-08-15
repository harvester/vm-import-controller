package source

import (
	"context"
	"fmt"
	"github.com/harvester/vm-import-controller/pkg/apis/common"
	source "github.com/harvester/vm-import-controller/pkg/apis/source.harvesterhci.io/v1beta1"
	sourceController "github.com/harvester/vm-import-controller/pkg/generated/controllers/source.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/source/openstack"
	"github.com/harvester/vm-import-controller/pkg/util"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

type openstackHandler struct {
	ctx    context.Context
	os     sourceController.OpenstackController
	secret corecontrollers.SecretController
}

func RegisterOpenstackController(ctx context.Context, os sourceController.OpenstackController, secret corecontrollers.SecretController) {
	oHandler := &openstackHandler{
		ctx:    ctx,
		os:     os,
		secret: secret,
	}

	os.OnChange(ctx, "openstack-source-change", oHandler.OnSourceChange)
}

func (h *openstackHandler) OnSourceChange(key string, o *source.Openstack) (*source.Openstack, error) {
	if o == nil || o.DeletionTimestamp != nil {
		return o, nil
	}

	logrus.Infof("reconcilling openstack soure :%s", key)
	if o.Status.Status != source.ClusterReady {
		// process source logic
		secretObj, err := h.secret.Get(o.Spec.Credentials.Namespace, o.Spec.Credentials.Name, metav1.GetOptions{})
		if err != nil {
			return o, fmt.Errorf("error looking up secret for openstacksource: %v", err)
		}

		client, err := openstack.NewClient(h.ctx, o.Spec.EndpointAddress, o.Spec.Region, secretObj)
		if err != nil {
			return o, fmt.Errorf("error generating openstack client for openstack source: %s: %v", o.Name, err)
		}

		err = client.Verify()
		if err != nil {
			conds := []common.Condition{
				{
					Type:               source.ClusterErrorCondition,
					Status:             v1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				}, {
					Type:               source.ClusterReadyCondition,
					Status:             v1.ConditionFalse,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				},
			}

			o.Status.Conditions = util.MergeConditions(o.Status.Conditions, conds)
			o.Status.Status = source.ClusterNotReady
			return h.os.UpdateStatus(o)
		}

		conds := []common.Condition{
			{
				Type:               source.ClusterReadyCondition,
				Status:             v1.ConditionTrue,
				LastUpdateTime:     metav1.Now().Format(time.RFC3339),
				LastTransitionTime: metav1.Now().Format(time.RFC3339),
			}, {
				Type:               source.ClusterErrorCondition,
				Status:             v1.ConditionFalse,
				LastUpdateTime:     metav1.Now().Format(time.RFC3339),
				LastTransitionTime: metav1.Now().Format(time.RFC3339),
			},
		}

		o.Status.Conditions = util.MergeConditions(o.Status.Conditions, conds)
		o.Status.Status = source.ClusterReady
		return h.os.UpdateStatus(o)

	}
	return o, nil
}
