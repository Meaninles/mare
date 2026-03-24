package catalog

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type AssetStatus string

const (
	AssetStatusReady         AssetStatus = "ready"
	AssetStatusPartial       AssetStatus = "partial"
	AssetStatusProcessing    AssetStatus = "processing"
	AssetStatusConflict      AssetStatus = "conflict"
	AssetStatusPendingDelete AssetStatus = "pending_delete"
	AssetStatusDeleted       AssetStatus = "deleted"
)

type ReplicaStatus string

const (
	ReplicaStatusActive        ReplicaStatus = "ACTIVE"
	ReplicaStatusMissing       ReplicaStatus = "MISSING"
	ReplicaStatusRestoring     ReplicaStatus = "RESTORING"
	ReplicaStatusProcessing    ReplicaStatus = "PROCESSING"
	ReplicaStatusConflict      ReplicaStatus = "CONFLICT"
	ReplicaStatusPendingDelete ReplicaStatus = "PENDING_DELETE"
	ReplicaStatusDeleted       ReplicaStatus = "DELETED"
)

type ReplicaStatusSnapshot struct {
	ReplicaStatus string
	ExistsFlag    bool
}

type ReplicaDiffInput struct {
	ReplicaID     string
	EndpointID    string
	ReplicaStatus string
	ExistsFlag    bool
	Size          *int64
	MTime         *time.Time
}

type ReplicaDiffResult struct {
	MissingEndpointIDs    []string
	ConsistentEndpointIDs []string
	UpdatedEndpointIDs    []string
	ConflictEndpointIDs   []string
}

type replicaLifecycle string

const (
	replicaLifecycleAvailable     replicaLifecycle = "available"
	replicaLifecycleMissing       replicaLifecycle = "missing"
	replicaLifecycleProcessing    replicaLifecycle = "processing"
	replicaLifecycleConflict      replicaLifecycle = "conflict"
	replicaLifecyclePendingDelete replicaLifecycle = "pending_delete"
	replicaLifecycleDeleted       replicaLifecycle = "deleted"
)

func AggregateAssetStatus(replicas []ReplicaStatusSnapshot) AssetStatus {
	if len(replicas) == 0 {
		return AssetStatusDeleted
	}

	hasAvailable := false
	hasProcessing := false
	hasConflict := false
	hasPendingDelete := false
	hasUnavailable := false

	for _, replica := range replicas {
		switch normalizeReplicaLifecycle(replica.ReplicaStatus, replica.ExistsFlag) {
		case replicaLifecycleAvailable:
			hasAvailable = true
		case replicaLifecycleProcessing:
			hasProcessing = true
		case replicaLifecycleConflict:
			hasConflict = true
		case replicaLifecyclePendingDelete:
			hasPendingDelete = true
		case replicaLifecycleMissing, replicaLifecycleDeleted:
			hasUnavailable = true
		}
	}

	switch {
	case hasConflict:
		return AssetStatusConflict
	case hasProcessing:
		return AssetStatusProcessing
	case hasAvailable && (hasPendingDelete || hasUnavailable):
		return AssetStatusPartial
	case hasAvailable:
		return AssetStatusReady
	case hasPendingDelete:
		return AssetStatusPendingDelete
	default:
		return AssetStatusDeleted
	}
}

func AnalyzeReplicaDifferences(expectedEndpointIDs []string, replicas []ReplicaDiffInput) ReplicaDiffResult {
	result := ReplicaDiffResult{
		MissingEndpointIDs:    []string{},
		ConsistentEndpointIDs: []string{},
		UpdatedEndpointIDs:    []string{},
		ConflictEndpointIDs:   []string{},
	}

	presentByEndpoint := make(map[string]struct{})
	comparableGroups := make(map[string][]ReplicaDiffInput)
	consistentEndpoints := make(map[string]struct{})
	conflictEndpoints := make(map[string]struct{})
	updatedEndpoints := make(map[string]struct{})

	comparableReplicas := make([]ReplicaDiffInput, 0, len(replicas))

	for _, replica := range replicas {
		if replicaIsPresent(replica) {
			presentByEndpoint[replica.EndpointID] = struct{}{}
		}
		if !replicaIsComparable(replica) {
			continue
		}

		comparableReplicas = append(comparableReplicas, replica)
		signature := replicaSignature(replica)
		comparableGroups[signature] = append(comparableGroups[signature], replica)
	}

	for _, endpointID := range uniqueStrings(expectedEndpointIDs) {
		if _, ok := presentByEndpoint[endpointID]; ok {
			continue
		}
		result.MissingEndpointIDs = append(result.MissingEndpointIDs, endpointID)
	}

	if len(comparableGroups) == 0 {
		return result
	}

	for _, group := range comparableGroups {
		if len(group) < 2 {
			continue
		}
		for _, replica := range group {
			consistentEndpoints[replica.EndpointID] = struct{}{}
		}
	}

	if len(comparableGroups) == 1 {
		result.ConsistentEndpointIDs = sortedSetValues(consistentEndpoints)
		return result
	}

	latestSignature, hasLatestSignature, ambiguous := findLatestSignature(comparableGroups)
	switch {
	case ambiguous:
		for _, replica := range comparableReplicas {
			conflictEndpoints[replica.EndpointID] = struct{}{}
		}
	case hasLatestSignature:
		for _, replica := range comparableGroups[latestSignature] {
			updatedEndpoints[replica.EndpointID] = struct{}{}
		}
	default:
		for _, replica := range comparableReplicas {
			conflictEndpoints[replica.EndpointID] = struct{}{}
		}
	}

	result.ConsistentEndpointIDs = sortedSetValues(consistentEndpoints)
	result.UpdatedEndpointIDs = sortedSetValues(updatedEndpoints)
	result.ConflictEndpointIDs = sortedSetValues(conflictEndpoints)
	return result
}

func normalizeReplicaLifecycle(status string, existsFlag bool) replicaLifecycle {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case string(ReplicaStatusActive), "READY", "AVAILABLE", "HEALTHY":
		return replicaLifecycleAvailable
	case string(ReplicaStatusRestoring), string(ReplicaStatusProcessing), "COPYING", "SCANNING", "UPLOADING", "DOWNLOADING":
		return replicaLifecycleProcessing
	case string(ReplicaStatusConflict):
		return replicaLifecycleConflict
	case string(ReplicaStatusPendingDelete), "DELETE_PENDING":
		return replicaLifecyclePendingDelete
	case string(ReplicaStatusDeleted):
		return replicaLifecycleDeleted
	case string(ReplicaStatusMissing), "NOT_FOUND":
		return replicaLifecycleMissing
	default:
		if existsFlag {
			return replicaLifecycleAvailable
		}
		return replicaLifecycleMissing
	}
}

func replicaIsPresent(replica ReplicaDiffInput) bool {
	lifecycle := normalizeReplicaLifecycle(replica.ReplicaStatus, replica.ExistsFlag)
	switch lifecycle {
	case replicaLifecycleAvailable, replicaLifecycleProcessing, replicaLifecycleConflict, replicaLifecyclePendingDelete:
		return true
	default:
		return false
	}
}

func replicaIsComparable(replica ReplicaDiffInput) bool {
	lifecycle := normalizeReplicaLifecycle(replica.ReplicaStatus, replica.ExistsFlag)
	if lifecycle != replicaLifecycleAvailable && lifecycle != replicaLifecycleConflict && lifecycle != replicaLifecycleProcessing {
		return false
	}
	return replica.Size != nil || replica.MTime != nil
}

func replicaSignature(replica ReplicaDiffInput) string {
	sizeValue := "unknown"
	if replica.Size != nil {
		sizeValue = fmt.Sprintf("%d", *replica.Size)
	}

	mtimeValue := "unknown"
	if replica.MTime != nil {
		mtimeValue = replica.MTime.UTC().Format(time.RFC3339Nano)
	}

	return sizeValue + "|" + mtimeValue
}

func findLatestSignature(groups map[string][]ReplicaDiffInput) (string, bool, bool) {
	type versionGroup struct {
		signature string
		replicas  []ReplicaDiffInput
		mtime     *time.Time
	}

	versions := make([]versionGroup, 0, len(groups))
	for signature, replicas := range groups {
		group := versionGroup{
			signature: signature,
			replicas:  replicas,
			mtime:     replicas[0].MTime,
		}
		versions = append(versions, group)
	}

	for _, version := range versions {
		if version.mtime == nil {
			return "", false, true
		}
	}

	sort.Slice(versions, func(left, right int) bool {
		if versions[left].mtime.Equal(*versions[right].mtime) {
			return versions[left].signature < versions[right].signature
		}
		return versions[left].mtime.After(*versions[right].mtime)
	})

	if len(versions) == 0 {
		return "", false, false
	}

	latest := versions[0]
	if len(versions) == 1 {
		return latest.signature, true, false
	}

	if latest.mtime.Equal(*versions[1].mtime) && latest.signature != versions[1].signature {
		return "", false, true
	}

	return latest.signature, true, false
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func sortedSetValues(values map[string]struct{}) []string {
	if len(values) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
