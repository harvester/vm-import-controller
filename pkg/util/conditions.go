package util

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
)

func GetCondition(conditions []common.Condition, c condition.Cond, condType corev1.ConditionStatus) *common.Condition {
	for _, v := range conditions {
		if v.Type == c && v.Status == condType {
			return &v
		}
	}

	return nil
}

func ConditionExists(conditions []common.Condition, c condition.Cond, condType corev1.ConditionStatus) bool {
	for _, v := range conditions {
		if v.Type == c && v.Status == condType {
			return true
		}
	}

	return false
}

func AddOrUpdateCondition(conditions []common.Condition, newCond common.Condition) []common.Condition {
	found := false

	for i := range conditions {
		v := &conditions[i]
		if v.Type == newCond.Type {
			found = true
			v.Status = newCond.Status
			v.LastTransitionTime = newCond.LastTransitionTime
			v.LastUpdateTime = newCond.LastUpdateTime
			v.Message = newCond.Message
			v.Reason = newCond.Reason
		}
	}

	if !found {
		conditions = append(conditions, newCond)
	}

	return conditions
}

func MergeConditions(srcConditions []common.Condition, newCond []common.Condition) []common.Condition {
	for _, v := range newCond {
		srcConditions = AddOrUpdateCondition(srcConditions, v)
	}

	return srcConditions
}

func RemoveCondition(conditions []common.Condition, c condition.Cond, condType corev1.ConditionStatus) []common.Condition {
	var retConditions []common.Condition
	for _, v := range conditions {
		if v.Type != c || v.Status != condType {
			retConditions = append(retConditions, v)
		}
	}
	return retConditions
}
