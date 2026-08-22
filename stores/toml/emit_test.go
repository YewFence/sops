package toml

import (
	"strings"
	"testing"

	"github.com/YewFence/sops/v3"
	tomllib "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmitPlainFileCanonicalAndStable(t *testing.T) {
	input := []byte(`# root
first = 1 # first inline
values = [ # array open
  # before value
  2, # value inline
  { # table open
    nested = true, # nested inline
  }, # table inline
] # array inline

# table
[service] # service inline
name = "api" # name inline

# servers
[[servers]] # first server
name = "one"
# second server
[[servers]] # second server
name = "two"
`)
	store := &Store{}
	firstTree, err := store.LoadPlainFile(input)
	require.NoError(t, err)
	firstOutput, err := store.EmitPlainFile(firstTree)
	require.NoError(t, err)
	assert.Equal(t, `# root
first = 1 # first inline
values = [ # array open
  # before value
  2, # value inline
  { # table open
    nested = true, # nested inline
  }, # table inline
] # array inline

# table
[service] # service inline
name = 'api' # name inline

# servers
[[servers]] # first server
name = 'one'

# second server
[[servers]] # second server
name = 'two'
`, string(firstOutput))
	secondTree, err := store.LoadPlainFile(firstOutput)
	require.NoError(t, err)
	secondOutput, err := store.EmitPlainFile(secondTree)
	require.NoError(t, err)
	assert.Equal(t, firstTree, secondTree)
	assert.Equal(t, string(firstOutput), string(secondOutput))
	var decoded map[string]any
	require.NoError(t, tomllib.Unmarshal(firstOutput, &decoded))
}

func TestEmitPlainFilePreservesInterleavedSectionOrder(t *testing.T) {
	store := &Store{}
	branches, err := store.LoadPlainFile([]byte(`[a.b]
x = 1
[c]
y = 2
[a.d]
z = 3
`))
	require.NoError(t, err)
	out, err := store.EmitPlainFile(branches)
	require.NoError(t, err)
	text := string(out)
	aB := strings.Index(text, "[a.b]")
	c := strings.Index(text, "[c]")
	aD := strings.Index(text, "[a.d]")
	assert.True(t, aB >= 0 && aB < c && c < aD, text)
}

func TestEmitPlainFileQuotesSpecialKeys(t *testing.T) {
	store := &Store{}
	branches := sops.TreeBranches{sops.TreeBranch{
		{Key: "a.b", Value: int64(1)},
		{Key: "a b", Value: int64(2)},
		{Key: "", Value: int64(3)},
		{Key: "123", Value: int64(4)},
		{Key: "雪", Value: int64(5)},
	}}
	out, err := store.EmitPlainFile(branches)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, tomllib.Unmarshal(out, &decoded))
	assert.Equal(t, int64(1), decoded["a.b"])
	assert.Equal(t, int64(2), decoded["a b"])
	assert.Equal(t, int64(3), decoded[""])
	assert.Equal(t, int64(4), decoded["123"])
	assert.Equal(t, int64(5), decoded["雪"])
}

func TestEmitPlainFileNormalizesBranchesAndArraysOfTables(t *testing.T) {
	store := &Store{}
	branches := sops.TreeBranches{sops.TreeBranch{
		{Key: "root", Value: "value"},
		{Key: "table", Value: sops.TreeBranch{
			{Key: sops.Comment{Value: "header", Inline: true}},
			{Key: "enabled", Value: true},
		}},
		{Key: sops.Comment{Value: "array tables"}},
		{Key: "servers", Value: []any{
			sops.TreeBranch{{Key: "name", Value: "one"}},
			sops.Comment{Value: "second"},
			sops.TreeBranch{{Key: "name", Value: "two"}},
		}},
	}}
	out, err := store.EmitPlainFile(branches)
	require.NoError(t, err)
	assert.Contains(t, string(out), "[table] # header")
	assert.Contains(t, string(out), "# array tables\n[[servers]]")
	assert.Contains(t, string(out), "# second\n[[servers]]")
	parsed, err := store.LoadPlainFile(out)
	require.NoError(t, err)
	second, err := store.EmitPlainFile(parsed)
	require.NoError(t, err)
	assert.Equal(t, string(out), string(second))
}

func TestEmitPlainFileKeepsEncryptedArrayTableCommentsInPlace(t *testing.T) {
	encryptedComment := "ENC[AES256_GCM,data:YQ==,iv:Yg==,tag:Yw==,type:comment]"
	branches := sops.TreeBranches{sops.TreeBranch{
		{Key: "service", Value: sops.TreeBranch{{Key: "name", Value: "api"}}},
		{Key: "servers", Value: []any{
			sops.TreeBranch{{Key: "name", Value: "one"}},
			encryptedComment,
			sops.TreeBranch{{Key: "name", Value: "two"}},
		}},
	}}
	store := &Store{}
	out, err := store.EmitPlainFile(branches)
	require.NoError(t, err)
	text := string(out)
	assert.Less(t, strings.Index(text, "[service]"), strings.Index(text, "[[servers]]"))
	assert.Contains(t, text, "# "+encryptedComment+"\n[[servers]]")
	loaded, err := store.LoadPlainFile(out)
	require.NoError(t, err)
	servers := loaded[0][1].Value.([]any)
	assert.Equal(t, sops.Comment{Value: encryptedComment}, servers[1])
}

func TestEmitValue(t *testing.T) {
	store := &Store{}
	tests := []struct {
		name  string
		value any
	}{
		{name: "scalar", value: "hello"},
		{name: "array", value: []any{int64(1), sops.Comment{Value: "one", Inline: true}, int64(2)}},
		{name: "branch", value: sops.TreeBranch{
			{Key: sops.Comment{Value: "open", Inline: true}},
			{Key: "first", Value: int64(1)},
			{Key: sops.Comment{Value: "first", Inline: true}},
			{Key: "nested", Value: sops.TreeBranch{{Key: "value", Value: true}}},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := store.EmitValue(test.value)
			require.NoError(t, err)
			var decoded map[string]any
			require.NoError(t, tomllib.Unmarshal([]byte("value = "+string(out)), &decoded))
		})
	}
}

func TestEmitPlainFileHandlesEmptyDocuments(t *testing.T) {
	store := &Store{}
	out, err := store.EmitPlainFile(nil)
	require.NoError(t, err)
	assert.Empty(t, out)
	out, err = store.EmitPlainFile(sops.TreeBranches{sops.TreeBranch{}})
	require.NoError(t, err)
	assert.Empty(t, out)
	out, err = store.EmitPlainFile(sops.TreeBranches{sops.TreeBranch{
		{Key: sops.Comment{Value: "comment"}},
	}})
	require.NoError(t, err)
	assert.Equal(t, "# comment\n", string(out))
}

func TestEmitPlainFileRejectsUnsupportedTrees(t *testing.T) {
	store := &Store{}
	_, err := store.EmitPlainFile(sops.TreeBranches{{}, {}})
	require.EqualError(t, err, "TOML cannot represent 2 documents in one file")
	_, err = store.EmitPlainFile(sops.TreeBranches{sops.TreeBranch{{Key: "nothing", Value: nil}}})
	require.Error(t, err)
	_, err = store.EmitValue(sops.TreeBranches{{}})
	require.Error(t, err)
}
