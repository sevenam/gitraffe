package tui

import (
	"reflect"
	"testing"
)

// syncModel builds a two-commit graph: commit A carries the given local tag refs.
func syncModel(refsOnA string, remote map[tagRef]bool) *model {
	m := &model{
		commits: []commit{
			{Hash: "aaaaaaa", FullHash: "a", Refs: refsOnA},
			{Hash: "bbbbbbb", FullHash: "b"},
		},
		displayRows: []displayRow{{CommitIdx: 0}, {CommitIdx: 1}},
		remoteTags:  remote,
	}
	m.applyRemoteTags()
	return m
}

func TestApplyRemoteTags(t *testing.T) {
	t.Run("tag on both is unmarked", func(t *testing.T) {
		m := syncModel("tag: refs/tags/v1.0", map[tagRef]bool{{Name: "v1.0", Commit: "a"}: true})
		if got := labelText(m.commits[0]); got != "v1.0" {
			t.Errorf("label = %q, want v1.0", got)
		}
	})

	t.Run("local-only tag is marked up", func(t *testing.T) {
		m := syncModel("tag: refs/tags/v1.0", map[tagRef]bool{})
		if got := labelText(m.commits[0]); got != "v1.0"+tagLocalOnlyMark {
			t.Errorf("label = %q, want v1.0%s", got, tagLocalOnlyMark)
		}
	})

	t.Run("remote-only tag is shown and marked down", func(t *testing.T) {
		m := syncModel("", map[tagRef]bool{{Name: "v1.0", Commit: "b"}: true})
		if got := labelText(m.commits[1]); got != "v1.0"+tagRemoteOnlyMark {
			t.Errorf("label = %q, want v1.0%s", got, tagRemoteOnlyMark)
		}
	})

	t.Run("moved tag shows at both ends", func(t *testing.T) {
		// Locally on A; the remote still has it on B.
		m := syncModel("tag: refs/tags/v1.0", map[tagRef]bool{{Name: "v1.0", Commit: "b"}: true})
		if got := labelText(m.commits[0]); got != "v1.0"+tagLocalOnlyMark {
			t.Errorf("new position = %q, want v1.0%s", got, tagLocalOnlyMark)
		}
		if got := labelText(m.commits[1]); got != "v1.0"+tagRemoteOnlyMark {
			t.Errorf("remote's position = %q, want v1.0%s", got, tagRemoteOnlyMark)
		}
	})

	t.Run("unknown remote state marks nothing", func(t *testing.T) {
		m := syncModel("tag: refs/tags/v1.0", nil)
		if got := labelText(m.commits[0]); got != "v1.0" {
			t.Errorf("label = %q, want unmarked v1.0 while the remote is unknown", got)
		}
	})

	t.Run("fallback loader without refs marks nothing", func(t *testing.T) {
		m := &model{
			commits:    []commit{{Hash: "bbbbbbb", FullHash: "b"}},
			remoteTags: map[tagRef]bool{{Name: "v1.0", Commit: "b"}: true},
		}
		m.applyRemoteTags()
		if got := labelText(m.commits[0]); got != "" {
			t.Errorf("label = %q, want empty: without refs every tag would look remote-only", got)
		}
	})

	t.Run("reapplying clears stale marks", func(t *testing.T) {
		m := syncModel("tag: refs/tags/v1.0", map[tagRef]bool{})
		m.remoteTags = map[tagRef]bool{{Name: "v1.0", Commit: "a"}: true}
		m.applyRemoteTags()
		if got := labelText(m.commits[0]); got != "v1.0" {
			t.Errorf("label = %q, want the stale mark cleared", got)
		}
	})

	t.Run("remote-only tags render in a stable order", func(t *testing.T) {
		remote := map[tagRef]bool{{Name: "v3", Commit: "b"}: true, {Name: "v1", Commit: "b"}: true, {Name: "v2", Commit: "b"}: true}
		for range 20 {
			m := syncModel("", remote)
			if got := m.commits[1].RemoteOnlyTags; !reflect.DeepEqual(got, []string{"v1", "v2", "v3"}) {
				t.Fatalf("RemoteOnlyTags = %v, want sorted", got)
			}
		}
	})
}

func TestRemoteTagsMsgWidensLabelColumn(t *testing.T) {
	m := testModel()
	m.commits = []commit{{Hash: "aaaaaaa", FullHash: "a", Refs: "tag: refs/tags/v1.0"}}
	m.displayRows = []displayRow{{CommitIdx: 0}}
	m.updateLabelWidth()
	if m.maxBranchWidth != 4 {
		t.Fatalf("width before = %d, want 4", m.maxBranchWidth)
	}

	res, _ := m.Update(remoteTagsMsg{repoPath: m.repoPath, tags: map[tagRef]bool{}})
	if got := res.(model).maxBranchWidth; got != 5 {
		t.Errorf("width after = %d, want 5 to fit the mark", got)
	}
}
