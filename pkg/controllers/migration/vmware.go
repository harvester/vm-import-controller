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
	"github.com/harvester/vm-import-controller/pkg/source/vmware"
	"github.com/harvester/vm-import-controller/pkg/util"
)

type vmwareHandler struct {
	ctx    context.Context
	vmware migrationController.VmwareSourceController
	secret corecontrollers.SecretController
}

func RegisterVmwareController(ctx context.Context, vc migrationController.VmwareSourceController, secret corecontrollers.SecretController) {
	vHandler := &vmwareHandler{
		ctx:    ctx,
		vmware: vc,
		secret: secret,
	}

	vc.OnChange(ctx, "vmware-migration-change", vHandler.OnSourceChange)
}

func (h *vmwareHandler) OnSourceChange(_ string, v *migration.VmwareSource) (*migration.VmwareSource, error) {
	if v == nil || v.DeletionTimestamp != nil {
		return v, nil
	}

	logrus.WithFields(logrus.Fields{
		"kind":      v.Kind,
		"name":      v.Name,
		"namespace": v.Namespace,
	}).Info("Reconciling source")

	if v.Status.Status != migration.ClusterReady {
		secretObj, err := h.secret.Get(v.Spec.Credentials.Namespace, v.Spec.Credentials.Name, metav1.GetOptions{})
		if err != nil {
			return v, fmt.Errorf("error looking up secret for vmware migration: %v", err)
		}

		client, err := vmware.NewClient(h.ctx, v.Spec.EndpointAddress, v.Spec.Datacenter, secretObj)
		if err != nil {
			return v, fmt.Errorf("error generating vmware client for vmware migration '%s': %v", v.Name, err)
		}

		err = client.Verify()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"apiVersion": v.APIVersion,
				"kind":       v.Kind,
				"name":       v.Name,
				"namespace":  v.Namespace,
				"err":        err,
			}).Error("failed to verify client for vmware migration")
			// unable to find specific datacenter
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

			v.Status.Conditions = util.MergeConditions(v.Status.Conditions, conds)
			v.Status.Status = migration.ClusterNotReady
			return h.vmware.UpdateStatus(v)
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

		v.Status.Conditions = util.MergeConditions(v.Status.Conditions, conds)
		v.Status.Status = migration.ClusterReady
		return h.vmware.UpdateStatus(v)
	}
	return v, nil
}
