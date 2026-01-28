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
	"github.com/harvester/vm-import-controller/pkg/source/kvm"
	"github.com/harvester/vm-import-controller/pkg/util"
)

type kvmHandler struct {
	ctx    context.Context
	source migrationController.KVMSourceController
	secret corecontrollers.SecretController
}

func RegisterKVMController(ctx context.Context, source migrationController.KVMSourceController, secret corecontrollers.SecretController) {
	kHandler := &kvmHandler{
		ctx:    ctx,
		source: source,
		secret: secret,
	}
	source.OnChange(ctx, "kvm-source-change", kHandler.OnSourceChange)
}

func (h *kvmHandler) OnSourceChange(_ string, s *migration.KVMSource) (*migration.KVMSource, error) {
	if s == nil || s.DeletionTimestamp != nil {
		return nil, nil
	}

	logrus.WithFields(logrus.Fields{
		"kind":      s.Kind,
		"name":      s.Name,
		"namespace": s.Namespace,
	}).Info("Reconciling source")

	if s.Status.Status != migration.ClusterReady {
		var client *kvm.Client
		var err error

		secretObj, err := h.secret.Get(s.SecretReference().Namespace, s.SecretReference().Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to lookup secret for %s migration %s: %w", s.Kind, s.NamespacedName(), err)
		}

		client, err = kvm.NewClient(h.ctx, s.Spec.LibvirtURI, secretObj, s.GetOptions().(migration.KVMSourceOptions))
		if err != nil {
			return nil, fmt.Errorf("failed to generate client for %s migration %s: %w", s.Kind, s.NamespacedName(), err)
		}
		defer client.Close() //nolint:errcheck

		if err := client.Verify(); err != nil {
			logrus.WithFields(logrus.Fields{
				"kind":      s.Kind,
				"name":      s.Name,
				"namespace": s.Namespace,
				"err":       err,
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

			s.Status.Conditions = util.MergeConditions(s.Status.Conditions, conds)
			s.Status.Status = migration.ClusterNotReady
			return h.source.UpdateStatus(s)
		}

		// Verify connection (NewClient already dials SSH, but we can run a simple command to be sure)
		// We can reuse PreFlightChecks logic or just run a simple command
		// But NewClient already dials, so if it succeeds, we are connected.
		// Let's just assume ready if NewClient succeeded.
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

		return h.source.UpdateStatus(s)
	}

	return nil, nil
}
