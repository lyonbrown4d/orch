package v1alpha1

import "strings"

func (r WorkloadRef) EffectiveCondition() DependencyCondition {
	return NormalizeDependencyCondition(r.Condition)
}

func NormalizeDependencyCondition(condition DependencyCondition) DependencyCondition {
	condition = DependencyCondition(strings.TrimSpace(string(condition)))
	if condition == "" {
		return DependencyConditionReady
	}
	return condition
}

func IsDependencyCondition(condition DependencyCondition) bool {
	switch NormalizeDependencyCondition(condition) {
	case DependencyConditionStarted, DependencyConditionReady, DependencyConditionCompleted:
		return true
	default:
		return false
	}
}

func WorkloadKindCanComplete(kind WorkloadKind) bool {
	switch kind {
	case WorkloadKindJob, WorkloadKindCron:
		return true
	case WorkloadKindService, WorkloadKindWorker, WorkloadKindStateful:
		return false
	default:
		return false
	}
}
