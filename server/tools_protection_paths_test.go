package server

import (
	"slices"
	"testing"
	"time"
)

// TestProtectionNarrowDatasets pins the dataset scope rule: a dataset argument
// keeps the dataset itself and its descendants, never a sibling whose name only
// starts with the same text.
func TestProtectionNarrowDatasets(t *testing.T) {
	all := []protectionDataset{
		{Name: "tank"},
		{Name: "tank/data"},
		{Name: "tank/data/sub"},
		{Name: "tank/data/sub/deep"},
		{Name: "tank/data2"},
		{Name: "tank/database"},
		{Name: "tankx/data"},
		{Name: "other/data"},
	}
	cases := []struct {
		name    string
		pool    string
		dataset string
		want    []string
	}{
		{"no scope", "", "", []string{"tank", "tank/data", "tank/data/sub", "tank/data/sub/deep", "tank/data2", "tank/database", "tankx/data", "other/data"}},
		{"pool only", "tank", "", []string{"tank", "tank/data", "tank/data/sub", "tank/data/sub/deep", "tank/data2", "tank/database"}},
		{"dataset and its descendants", "tank", "tank/data", []string{"tank/data", "tank/data/sub", "tank/data/sub/deep"}},
		{"dataset without pool", "", "tank/data", []string{"tank/data", "tank/data/sub", "tank/data/sub/deep"}},
		{"leaf dataset", "tank", "tank/data/sub/deep", []string{"tank/data/sub/deep"}},
		{"pool root as dataset", "tank", "tank", []string{"tank", "tank/data", "tank/data/sub", "tank/data/sub/deep", "tank/data2", "tank/database"}},
		{"unknown dataset", "tank", "tank/dat", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := []string{}
			for _, d := range protectionNarrowDatasets(all, tc.pool, tc.dataset) {
				got = append(got, d.Name)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("protectionNarrowDatasets(%q, %q) = %v, want %v", tc.pool, tc.dataset, got, tc.want)
			}
		})
	}
}

// TestProtectionReplicaTargetBoundary pins the replica rule: a local
// replication target makes the target and its descendants replicas, and a
// sibling whose name only starts with the target's name is not one.
func TestProtectionReplicaTargetBoundary(t *testing.T) {
	in := protectionInputs{
		replications: []protectionReplication{
			{ID: 7, Direction: "PUSH", Transport: "LOCAL", SourceDatasets: []string{"fast/life"}, TargetDataset: "big/replica", Enabled: true},
			{ID: 8, Direction: "PULL", Transport: "SSH", TargetDataset: "big/pulled", Enabled: true},
			{ID: 9, Direction: "PUSH", Transport: "SSH", SourceDatasets: []string{"fast/life"}, TargetDataset: "big/remote", Enabled: true},
		},
	}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		dataset string
		want    []int
	}{
		{"big/replica", []int{7}},
		{"big/replica/life", []int{7}},
		{"big/replica/life/deep", []int{7}},
		{"big/replica2", nil},
		{"big/replicas/life", nil},
		{"big", nil},
		{"big/pulled", []int{8}},
		{"big/pulled/x", []int{8}},
		{"big/pulledx", nil},
		{"big/remote", nil},
	}
	for _, tc := range cases {
		t.Run(tc.dataset, func(t *testing.T) {
			row := protectionDatasetRowFor(protectionDataset{Name: tc.dataset}, in, map[int]protectionSnapshotTask{}, nil, now)
			if !slices.Equal(row.ReplicaOf, tc.want) {
				t.Errorf("replica_of of %q = %v, want %v", tc.dataset, row.ReplicaOf, tc.want)
			}
		})
	}
}

// TestTaskCoversExcludeBoundary pins the exclude rule: an exclude entry hides
// the entry and its descendants, never a sibling whose name only starts with
// the entry's name.
func TestTaskCoversExcludeBoundary(t *testing.T) {
	cases := []struct {
		dataset      string
		wantCovered  bool
		wantExcluded bool
	}{
		{"tank/data/tmp", false, true},
		{"tank/data/tmp/deep", false, true},
		{"tank/data/tmp2", true, false},
		{"tank/data/tmpx/deep", true, false},
		{"tank/data", true, false},
	}
	exclude := []string{"", "tank/data/tmp"}
	for _, tc := range cases {
		t.Run(tc.dataset, func(t *testing.T) {
			covered, excluded := taskCovers("tank/data", true, exclude, tc.dataset)
			if covered != tc.wantCovered || excluded != tc.wantExcluded {
				t.Errorf("taskCovers(%q) = (%v, %v), want (%v, %v)", tc.dataset, covered, excluded, tc.wantCovered, tc.wantExcluded)
			}
		})
	}
}

// TestProtectionSortedKeys pins the order of the cron value lists.
func TestProtectionSortedKeys(t *testing.T) {
	cases := []struct {
		in   map[int]bool
		want []int
	}{
		{map[int]bool{}, []int{}},
		{map[int]bool{5: true}, []int{5}},
		{map[int]bool{59: true, 0: true, 30: true, 15: true, 45: true}, []int{0, 15, 30, 45, 59}},
		{map[int]bool{3: false, 1: true}, []int{1, 3}},
	}
	for _, tc := range cases {
		got := protectionSortedKeys(tc.in)
		if !slices.Equal(got, tc.want) {
			t.Errorf("protectionSortedKeys(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
