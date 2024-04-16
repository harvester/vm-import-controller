package migration

import (
	"context"
	"fmt"
	"time"

	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	migrationController "github.com/harvester/vm-import-controller/pkg/generated/controllers/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/source/openstack"
	"github.com/harvester/vm-import-controller/pkg/util"
)

type openstackHandler struct {
	ctx    context.Context
	os     migrationController.OpenstackSourceController
	secret corecontrollers.SecretController
}

func RegisterOpenstackController(ctx context.Context, os migrationController.OpenstackSourceController, secret corecontrollers.SecretController) {
	oHandler := &openstackHandler{
		ctx:    ctx,
		os:     os,
		secret: secret,
	}

	os.OnChange(ctx, "openstack-migration-change", oHandler.OnSourceChange)
}

func (h *openstackHandler) OnSourceChange(key string, o *migration.OpenstackSource) (*migration.OpenstackSource, error) {
	if o == nil || o.DeletionTimestamp != nil {
		return o, nil
	}

	logrus.Infof("reconcilling openstack soure :%s", key)
	if o.Status.Status != migration.ClusterReady {
		// process migration logic
		secretObj, err := h.secret.Get(o.Spec.Credentials.Namespace, o.Spec.Credentials.Name, metav1.GetOptions{})
		if err != nil {
			return o, fmt.Errorf("error looking up secret for openstacksource: %v", err)
		}

		client, err := openstack.NewClient(h.ctx, o.Spec.EndpointAddress, o.Spec.Region, secretObj)
		if err != nil {
			return o, fmt.Errorf("error generating openstack client for openstack migration: %s: %v", o.Name, err)
		}

		err = client.Verify()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"apiVersion": o.APIVersion,
				"kind":       o.Kind,
				"name":       o.Name,
				"namespace":  o.Namespace,
				"err":        err,
			}).Error("failed to verfiy client for openstack migration")
			conds := []common.Condition{
				{
					Type:               migration.ClusterErrorCondition,
					Status:             v1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				}, {
					Type:               migration.ClusterReadyCondition,
					Status:             v1.ConditionFalse,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				},
			}

			o.Status.Conditions = util.MergeConditions(o.Status.Conditions, conds)
			o.Status.Status = migration.ClusterNotReady
			return h.os.UpdateStatus(o)
		}

		conds := []common.Condition{
			{
				Type:               migration.ClusterReadyCondition,
				Status:             v1.ConditionTrue,
				LastUpdateTime:     metav1.Now().Format(time.RFC3339),
				LastTransitionTime: metav1.Now().Format(time.RFC3339),
			}, {
				Type:               migration.ClusterErrorCondition,
				Status:             v1.ConditionFalse,
				LastUpdateTime:     metav1.Now().Format(time.RFC3339),
				LastTransitionTime: metav1.Now().Format(time.RFC3339),
			},
		}

		o.Status.Conditions = util.MergeConditions(o.Status.Conditions, conds)
		o.Status.Status = migration.ClusterReady
		return h.os.UpdateStatus(o)

	}
	return o, nil
}
