package source

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
	"github.com/sirupsen/logrus"

	source "github.com/harvester/vm-import-controller/pkg/apis/source.harvesterhci.io/v1beta1"
	sourceController "github.com/harvester/vm-import-controller/pkg/generated/controllers/source.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/source/vmware"
	"github.com/harvester/vm-import-controller/pkg/util"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type vmwareHandler struct {
	ctx    context.Context
	vmware sourceController.VmwareController
	secret corecontrollers.SecretController
}

func RegisterVmareController(ctx context.Context, vc sourceController.VmwareController, secret corecontrollers.SecretController) {
	vHandler := &vmwareHandler{
		ctx:    ctx,
		vmware: vc,
		secret: secret,
	}

	vc.OnChange(ctx, "vmware-source-change", vHandler.OnSourceChange)
}

func (h *vmwareHandler) OnSourceChange(key string, v *source.Vmware) (*source.Vmware, error) {
	if v == nil || v.DeletionTimestamp != nil {
		return v, nil
	}

	logrus.Infof("reoncilling vmware source %s", key)
	if v.Status.Status != source.ClusterReady {
		secretObj, err := h.secret.Get(v.Spec.Credentials.Namespace, v.Spec.Credentials.Name, metav1.GetOptions{})
		if err != nil {
			return v, fmt.Errorf("error looking up secret for vmware source: %s", err)
		}
		client, err := vmware.NewClient(h.ctx, v.Spec.EndpointAddress, v.Spec.Datacenter, secretObj)
		if err != nil {
			return v, fmt.Errorf("error generating vmware client for vmware source: %s: %v", v.Name, err)
		}

		err = client.Verify()
		if err != nil {
			// unable to find specific datacenter
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

			v.Status.Conditions = util.MergeConditions(v.Status.Conditions, conds)
			v.Status.Status = source.ClusterNotReady
			return h.vmware.UpdateStatus(v)
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

		v.Status.Conditions = util.MergeConditions(v.Status.Conditions, conds)
		v.Status.Status = source.ClusterReady
		return h.vmware.UpdateStatus(v)
	}
	return v, nil
}
