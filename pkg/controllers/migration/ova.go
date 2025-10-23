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
	"github.com/harvester/vm-import-controller/pkg/source/ova"
	"github.com/harvester/vm-import-controller/pkg/util"
)

type ovaHandler struct {
	ctx    context.Context
	source migrationController.OvaSourceController
	secret corecontrollers.SecretController
}

func RegisterOvaController(ctx context.Context, source migrationController.OvaSourceController, secret corecontrollers.SecretController) {
	handler := &ovaHandler{
		ctx:    ctx,
		source: source,
		secret: secret,
	}
	source.OnChange(ctx, "ova-source-change", handler.OnSourceChange)
}

func (h *ovaHandler) OnSourceChange(_ string, s *migration.OvaSource) (*migration.OvaSource, error) {
	if s == nil || s.DeletionTimestamp != nil {
		return nil, nil
	}

	logrus.WithFields(logrus.Fields{
		"kind":      s.Kind,
		"name":      s.Name,
		"namespace": s.Namespace,
	}).Info("Reconciling source")

	if s.Status.Status != migration.ClusterReady {
		var secret *corev1.Secret

		if s.HasSecret() {
			var err error
			secret, err = h.secret.Get(s.SecretReference().Namespace, s.SecretReference().Name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to lookup secret for %s migration %s: %w", s.Kind, s.NamespacedName(), err)
			}
		}

		client, err := ova.NewClient(h.ctx, s.Spec.Url, secret, s.GetOptions().(migration.OvaSourceOptions))
		if err != nil {
			return nil, fmt.Errorf("failed to generate client for %s migration %s: %w", s.Kind, s.NamespacedName(), err)
		}

		err = client.Verify()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"apiVersion": s.APIVersion,
				"kind":       s.Kind,
				"name":       s.Name,
				"namespace":  s.Namespace,
				"err":        err,
			}).Error("Failed to verify source for migration")

			// unable to find specific datacenter
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

			s.Status.Conditions = util.MergeConditions(s.Status.Conditions, conds)
			s.Status.Status = migration.ClusterNotReady
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

			s.Status.Conditions = util.MergeConditions(s.Status.Conditions, conds)
			s.Status.Status = migration.ClusterReady
		}

		return h.source.UpdateStatus(s)
	}

	return nil, nil
}
