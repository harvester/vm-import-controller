package migration

import (
	"context"
	"fmt"
	"time"

	corecontrollers "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	migrationController "github.com/harvester/vm-import-controller/pkg/generated/controllers/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/source/openstack"
	"github.com/harvester/vm-import-controller/pkg/util"
)

type openstackHandler struct {
	ctx    context.Context
	source migrationController.OpenstackSourceController
	secret corecontrollers.SecretController
}

func RegisterOpenstackController(ctx context.Context, source migrationController.OpenstackSourceController, secret corecontrollers.SecretController) {
	oHandler := &openstackHandler{
		ctx:    ctx,
		source: source,
		secret: secret,
	}
	source.OnChange(ctx, "openstack-source-change", oHandler.OnSourceChange)
}

func (h *openstackHandler) OnSourceChange(_ string, o *migration.OpenstackSource) (*migration.OpenstackSource, error) {
	if o == nil || o.DeletionTimestamp != nil {
		return nil, nil
	}

	logrus.WithFields(logrus.Fields{
		"kind":      o.Kind,
		"name":      o.Name,
		"namespace": o.Namespace,
	}).Info("Reconciling source")

	if o.Status.Status != migration.ClusterReady {
		// process migration logic
		secretObj, err := h.secret.Get(o.SecretReference().Namespace, o.SecretReference().Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to lookup secret for %s migration %s: %w", o.Kind, o.NamespacedName(), err)
		}

		client, err := openstack.NewClient(h.ctx, o.Spec.EndpointAddress, o.Spec.Region, secretObj, o.GetOptions().(migration.OpenstackSourceOptions))
		if err != nil {
			return nil, fmt.Errorf("failed to generate client for %s migration %s: %w", o.Kind, o.NamespacedName(), err)
		}

		err = client.Verify()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"apiVersion": o.APIVersion,
				"kind":       o.Kind,
				"name":       o.Name,
				"namespace":  o.Namespace,
				"err":        err,
			}).Error("Failed to verify source for migration")

			conds := []common.Condition{
				{
					Type:               migration.ClusterErrorCondition,
					Status:             corev1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				}, {
					Type:               migration.ClusterReadyCondition,
					Status:             corev1.ConditionFalse,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				},
			}

			o.Status.Conditions = util.MergeConditions(o.Status.Conditions, conds)
			o.Status.Status = migration.ClusterNotReady
		} else {
			conds := []common.Condition{
				{
					Type:               migration.ClusterReadyCondition,
					Status:             corev1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				}, {
					Type:               migration.ClusterErrorCondition,
					Status:             corev1.ConditionFalse,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				},
			}

			o.Status.Conditions = util.MergeConditions(o.Status.Conditions, conds)
			o.Status.Status = migration.ClusterReady
		}

		return h.source.UpdateStatus(o)
	}

	return nil, nil
}
