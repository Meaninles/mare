package catalog

import (
	"testing"
	"time"
)

func TestAggregateAssetStatus(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		replicas []ReplicaStatusSnapshot
		expected AssetStatus
	}{
		{
			name: "single available replica keeps asset ready",
			replicas: []ReplicaStatusSnapshot{
				{ReplicaStatus: string(ReplicaStatusActive), ExistsFlag: true},
			},
			expected: AssetStatusReady,
		},
		{
			name: "available and missing replica becomes partial",
			replicas: []ReplicaStatusSnapshot{
				{ReplicaStatus: string(ReplicaStatusActive), ExistsFlag: true},
				{ReplicaStatus: string(ReplicaStatusMissing), ExistsFlag: false},
			},
			expected: AssetStatusPartial,
		},
		{
			name: "processing replica dominates ready state",
			replicas: []ReplicaStatusSnapshot{
				{ReplicaStatus: string(ReplicaStatusProcessing), ExistsFlag: true},
				{ReplicaStatus: string(ReplicaStatusActive), ExistsFlag: true},
			},
			expected: AssetStatusProcessing,
		},
		{
			name: "conflict replica dominates aggregate result",
			replicas: []ReplicaStatusSnapshot{
				{ReplicaStatus: string(ReplicaStatusConflict), ExistsFlag: true},
				{ReplicaStatus: string(ReplicaStatusActive), ExistsFlag: true},
			},
			expected: AssetStatusConflict,
		},
		{
			name: "pending delete without available replicas stays pending delete",
			replicas: []ReplicaStatusSnapshot{
				{ReplicaStatus: string(ReplicaStatusPendingDelete), ExistsFlag: true},
			},
			expected: AssetStatusPendingDelete,
		},
		{
			name: "all missing replicas become deleted",
			replicas: []ReplicaStatusSnapshot{
				{ReplicaStatus: string(ReplicaStatusMissing), ExistsFlag: false},
				{ReplicaStatus: string(ReplicaStatusDeleted), ExistsFlag: false},
			},
			expected: AssetStatusDeleted,
		},
		{
			name:     "no replicas becomes deleted",
			replicas: nil,
			expected: AssetStatusDeleted,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual := AggregateAssetStatus(testCase.replicas)
			if actual != testCase.expected {
				t.Fatalf("expected %s, got %s", testCase.expected, actual)
			}
		})
	}
}

func TestAnalyzeReplicaDifferences(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2026, time.March, 24, 10, 0, 0, 0, time.UTC)
	olderTime := baseTime.Add(-2 * time.Hour)
	laterTime := baseTime.Add(15 * time.Minute)

	testCases := []struct {
		name               string
		expectedEndpoints  []string
		replicas           []ReplicaDiffInput
		expectedMissing    []string
		expectedConsistent []string
		expectedUpdated    []string
		expectedConflicts  []string
	}{
		{
			name:              "missing endpoint is detected",
			expectedEndpoints: []string{"local", "qnap"},
			replicas: []ReplicaDiffInput{
				{
					ReplicaID:     "replica-local",
					EndpointID:    "local",
					ReplicaStatus: string(ReplicaStatusActive),
					ExistsFlag:    true,
					Size:          int64Pointer(100),
					MTime:         &baseTime,
				},
			},
			expectedMissing: []string{"qnap"},
		},
		{
			name: "identical replicas are marked consistent",
			replicas: []ReplicaDiffInput{
				{
					ReplicaID:     "replica-local",
					EndpointID:    "local",
					ReplicaStatus: string(ReplicaStatusActive),
					ExistsFlag:    true,
					Size:          int64Pointer(100),
					MTime:         &baseTime,
				},
				{
					ReplicaID:     "replica-qnap",
					EndpointID:    "qnap",
					ReplicaStatus: string(ReplicaStatusActive),
					ExistsFlag:    true,
					Size:          int64Pointer(100),
					MTime:         &baseTime,
				},
			},
			expectedConsistent: []string{"local", "qnap"},
		},
		{
			name: "latest replica is marked updated",
			replicas: []ReplicaDiffInput{
				{
					ReplicaID:     "replica-local",
					EndpointID:    "local",
					ReplicaStatus: string(ReplicaStatusActive),
					ExistsFlag:    true,
					Size:          int64Pointer(100),
					MTime:         &olderTime,
				},
				{
					ReplicaID:     "replica-qnap",
					EndpointID:    "qnap",
					ReplicaStatus: string(ReplicaStatusActive),
					ExistsFlag:    true,
					Size:          int64Pointer(120),
					MTime:         &laterTime,
				},
			},
			expectedUpdated: []string{"qnap"},
		},
		{
			name: "same mtime but different versions become conflict candidates",
			replicas: []ReplicaDiffInput{
				{
					ReplicaID:     "replica-local",
					EndpointID:    "local",
					ReplicaStatus: string(ReplicaStatusActive),
					ExistsFlag:    true,
					Size:          int64Pointer(100),
					MTime:         &baseTime,
				},
				{
					ReplicaID:     "replica-qnap",
					EndpointID:    "qnap",
					ReplicaStatus: string(ReplicaStatusActive),
					ExistsFlag:    true,
					Size:          int64Pointer(160),
					MTime:         &baseTime,
				},
			},
			expectedConflicts: []string{"local", "qnap"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual := AnalyzeReplicaDifferences(testCase.expectedEndpoints, testCase.replicas)
			assertStringSliceEqual(t, "missing", testCase.expectedMissing, actual.MissingEndpointIDs)
			assertStringSliceEqual(t, "consistent", testCase.expectedConsistent, actual.ConsistentEndpointIDs)
			assertStringSliceEqual(t, "updated", testCase.expectedUpdated, actual.UpdatedEndpointIDs)
			assertStringSliceEqual(t, "conflict", testCase.expectedConflicts, actual.ConflictEndpointIDs)
		})
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func assertStringSliceEqual(t *testing.T, label string, expected, actual []string) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Fatalf("%s length mismatch: expected %v, got %v", label, expected, actual)
	}

	for index := range expected {
		if expected[index] != actual[index] {
			t.Fatalf("%s mismatch: expected %v, got %v", label, expected, actual)
		}
	}
}
