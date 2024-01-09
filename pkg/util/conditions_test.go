package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
)

func Test_ConditionExists(t *testing.T) {
	conditions := []common.Condition{
		{
			Type:               migration.ClusterReadyCondition,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
		{
			Type:               migration.ClusterErrorCondition,
			Status:             corev1.ConditionFalse,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
	}

	assert := require.New(t)
	assert.True(ConditionExists(conditions, migration.ClusterReadyCondition, corev1.ConditionTrue))
	assert.True(ConditionExists(conditions, migration.ClusterErrorCondition, corev1.ConditionFalse))
}

func Test_AddOrUpdateCondition(t *testing.T) {
	conditions := []common.Condition{
		{
			Type:               migration.ClusterReadyCondition,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
		{
			Type:               migration.ClusterErrorCondition,
			Status:             corev1.ConditionFalse,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
	}

	extraCondition := common.Condition{

		Type:               migration.VirtualMachinePoweringOff,
		Status:             corev1.ConditionTrue,
		LastUpdateTime:     metav1.Now().Format(time.RFC3339),
		LastTransitionTime: metav1.Now().Format(time.RFC3339),
	}

	newCond := AddOrUpdateCondition(conditions, extraCondition)
	assert := require.New(t)
	assert.True(ConditionExists(newCond, migration.VirtualMachinePoweringOff, corev1.ConditionTrue))
	assert.True(ConditionExists(conditions, migration.ClusterErrorCondition, corev1.ConditionFalse))
	assert.True(ConditionExists(conditions, migration.ClusterReadyCondition, corev1.ConditionTrue))
}

func Test_MergeConditions(t *testing.T) {
	conditions := []common.Condition{
		{
			Type:               migration.ClusterReadyCondition,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
		{
			Type:               migration.ClusterErrorCondition,
			Status:             corev1.ConditionFalse,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
	}

	extraConditions := []common.Condition{
		{
			Type:               migration.VirtualMachineExported,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
		{
			Type:               migration.VirtualMachineImageReady,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
	}

	newConds := MergeConditions(conditions, extraConditions)
	assert := require.New(t)
	assert.Len(newConds, 4, "expected to find 4 conditions in the merged conditions")
}

func Test_RemoveCondition(t *testing.T) {
	conditions := []common.Condition{
		{
			Type:               migration.ClusterReadyCondition,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
		{
			Type:               migration.ClusterErrorCondition,
			Status:             corev1.ConditionFalse,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
	}

	noRemoveCond := RemoveCondition(conditions, migration.ClusterErrorCondition, corev1.ConditionTrue)
	assert := require.New(t)
	assert.True(ConditionExists(noRemoveCond, migration.ClusterErrorCondition, corev1.ConditionFalse))
	removeCond := RemoveCondition(conditions, migration.ClusterErrorCondition, corev1.ConditionFalse)
	assert.False(ConditionExists(removeCond, migration.ClusterErrorCondition, corev1.ConditionFalse))
}
