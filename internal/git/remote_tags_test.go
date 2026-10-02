package git

import (
	"reflect"
	"sort"
	"testing"
)

func TestParseLsRemoteTags(t *testing.T) {
	out := "" +
		"1111111111111111111111111111111111111111\trefs/tags/v1.0\n" +
		// Annotated: the tag object first, then peeled to the commit.
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\trefs/tags/v2.0\r\n" +
		"2222222222222222222222222222222222222222\trefs/tags/v2.0^{}\r\n" +
		"3333333333333333333333333333333333333333\trefs/heads/main\n" +
		"\n"

	got := parseLsRemoteTags(out)
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })

	want := []TagRef{
		{Name: "v1.0", Commit: "1111111111111111111111111111111111111111"},
		// The peeled commit, not the tag object — that is what the graph shows.
		{Name: "v2.0", Commit: "2222222222222222222222222222222222222222"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestParseLsRemoteTagsPeeledWinsInAnyOrder(t *testing.T) {
	out := "2222222222222222222222222222222222222222\trefs/tags/v2.0^{}\n" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\trefs/tags/v2.0\n"

	got := parseLsRemoteTags(out)
	if len(got) != 1 || got[0].Commit != "2222222222222222222222222222222222222222" {
		t.Errorf("got %+v, want the peeled commit", got)
	}
}
