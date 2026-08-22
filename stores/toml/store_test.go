package toml

import (
	"math"
	"testing"
	"time"

	"github.com/YewFence/sops/v3"
	tomllib "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPlainFilePreservesOrderAndScalarTypes(t *testing.T) {
	input := []byte(`basic = "hello\nworld"
literal = 'C:\Users\nodejs\templates'
multiline_basic = """
roses are red
violets are blue"""
multiline_literal = '''
The first newline is
trimmed in raw strings.'''
decimal = 42
hex = 0x2A
float = 6.626e-34
positive_inf = +inf
negative_inf = -inf
not_a_number = nan
enabled = true
offset_datetime = 1979-05-27T07:32:00-07:00
local_datetime = 1979-05-27T07:32:00
local_date = 1979-05-27
local_time = 07:32:00
`)
	branches, err := (&Store{}).LoadPlainFile(input)
	require.NoError(t, err)
	require.Len(t, branches, 1)
	branch := branches[0]
	require.Len(t, branch, 15)
	assert.Equal(t, "basic", branch[0].Key)
	assert.Equal(t, "hello\nworld", branch[0].Value)
	assert.Equal(t, "literal", branch[1].Key)
	assert.Equal(t, `C:\Users\nodejs\templates`, branch[1].Value)
	assert.Equal(t, "multiline_basic", branch[2].Key)
	assert.Equal(t, "roses are red\nviolets are blue", branch[2].Value)
	assert.Equal(t, "multiline_literal", branch[3].Key)
	assert.Equal(t, "The first newline is\ntrimmed in raw strings.", branch[3].Value)
	assert.Equal(t, int64(42), branch[4].Value)
	assert.Equal(t, int64(42), branch[5].Value)
	assert.Equal(t, 6.626e-34, branch[6].Value)
	assert.True(t, math.IsInf(branch[7].Value.(float64), 1))
	assert.True(t, math.IsInf(branch[8].Value.(float64), -1))
	assert.True(t, math.IsNaN(branch[9].Value.(float64)))
	assert.Equal(t, true, branch[10].Value)
	assert.True(t, time.Date(1979, 5, 27, 7, 32, 0, 0, time.FixedZone("", -7*60*60)).Equal(branch[11].Value.(time.Time)))
	var expectedLocalDateTime tomllib.LocalDateTime
	require.NoError(t, expectedLocalDateTime.UnmarshalText([]byte("1979-05-27T07:32:00")))
	assert.Equal(t, expectedLocalDateTime, branch[12].Value)
	var expectedLocalDate tomllib.LocalDate
	require.NoError(t, expectedLocalDate.UnmarshalText([]byte("1979-05-27")))
	assert.Equal(t, expectedLocalDate, branch[13].Value)
	var expectedLocalTime tomllib.LocalTime
	require.NoError(t, expectedLocalTime.UnmarshalText([]byte("07:32:00")))
	assert.Equal(t, expectedLocalTime, branch[14].Value)
}

func TestLoadPlainFileEmitsPublishableMap(t *testing.T) {
	branches, err := (&Store{}).LoadPlainFile([]byte(`local_datetime = 1979-05-27T07:32:00
local_date = 1979-05-27
local_time = 07:32:00

[[servers]]
name = "one"

# second server
[[servers]]
name = "two"
`))
	require.NoError(t, err)

	data, err := sops.EmitAsMap(branches)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"local_datetime": "1979-05-27T07:32:00",
		"local_date":     "1979-05-27",
		"local_time":     "07:32:00",
		"servers": []interface{}{
			map[string]interface{}{"name": "one"},
			map[string]interface{}{"name": "two"},
		},
	}, data)
}

func TestLoadPlainFilePreservesComments(t *testing.T) {
	input := []byte(`# root comment
root = 1 # root inline
# table comment
[table] # table inline
# value comment
value = 2 # value inline
# eof comment
`)
	branches, err := (&Store{}).LoadPlainFile(input)
	require.NoError(t, err)
	require.Len(t, branches, 1)
	assert.Equal(t, sops.TreeBranch{
		{Key: sops.Comment{Value: "root comment"}},
		{Key: "root", Value: int64(1)},
		{Key: sops.Comment{Value: "root inline", Inline: true}},
		{Key: sops.Comment{Value: "table comment"}},
		{Key: "table", Value: sops.TreeBranch{
			{Key: sops.Comment{Value: "table inline", Inline: true}},
			{Key: sops.Comment{Value: "value comment"}},
			{Key: "value", Value: int64(2)},
			{Key: sops.Comment{Value: "value inline", Inline: true}},
			{Key: sops.Comment{Value: "eof comment"}},
		}},
	}, branches[0])
}

func TestLoadPlainFilePreservesArrayAndInlineTableComments(t *testing.T) {
	input := []byte(`values = [ # after open
  # before first
  1, # after first
  { # after table open
    "a.b" = true, # after dotted key
    # before nested key
    nested.key = "value",
  }, # after table
] # after array
`)
	branches, err := (&Store{}).LoadPlainFile(input)
	require.NoError(t, err)
	assert.Equal(t, sops.TreeBranch{
		{Key: "values", Value: []any{
			sops.Comment{Value: "after open", Inline: true},
			sops.Comment{Value: "before first"},
			int64(1),
			sops.Comment{Value: "after first", Inline: true},
			sops.TreeBranch{
				{Key: sops.Comment{Value: "after table open", Inline: true}},
				{Key: "a.b", Value: true},
				{Key: sops.Comment{Value: "after dotted key", Inline: true}},
				{Key: "nested", Value: sops.TreeBranch{
					{Key: sops.Comment{Value: "before nested key"}},
					{Key: "key", Value: "value"},
				}},
			},
			sops.Comment{Value: "after table", Inline: true},
		}},
		{Key: sops.Comment{Value: "after array", Inline: true}},
	}, branches[0])
}

func TestLoadPlainFileAttachesInlineTableTrailingCommentToTable(t *testing.T) {
	branches, err := (&Store{}).LoadPlainFile([]byte(`inline = { z = "first", a = "second" } # inline table comment
`))
	require.NoError(t, err)
	assert.Equal(t, sops.TreeBranch{
		{Key: "inline", Value: sops.TreeBranch{
			{Key: sops.Comment{Value: "inline table comment", Inline: true}},
			{Key: "z", Value: "first"},
			{Key: "a", Value: "second"},
		}},
	}, branches[0])
}

func TestLoadPlainFilePreservesTablesAndArraysOfTables(t *testing.T) {
	input := []byte(`# first section
[a.b] # b header
x = 1
[c]
y = 2
[a.d]
z = 3
# servers
[[servers]] # first server
name = "one"
# second element
[[servers]] # second server
name = "two"
[[servers.features]]
enabled = true
`)
	branches, err := (&Store{}).LoadPlainFile(input)
	require.NoError(t, err)
	assert.Equal(t, sops.TreeBranch{
		{Key: "a", Value: sops.TreeBranch{
			{Key: sops.Comment{Value: "first section"}},
			{Key: "b", Value: sops.TreeBranch{
				{Key: sops.Comment{Value: "b header", Inline: true}},
				{Key: "x", Value: int64(1)},
			}},
		}},
		{Key: "c", Value: sops.TreeBranch{{Key: "y", Value: int64(2)}}},
		{Key: "a", Value: sops.TreeBranch{
			{Key: "d", Value: sops.TreeBranch{{Key: "z", Value: int64(3)}}},
		}},
		{Key: sops.Comment{Value: "servers"}},
		{Key: "servers", Value: []any{
			sops.TreeBranch{
				{Key: sops.Comment{Value: "first server", Inline: true}},
				{Key: "name", Value: "one"},
			},
			sops.Comment{Value: "second element"},
			sops.TreeBranch{
				{Key: sops.Comment{Value: "second server", Inline: true}},
				{Key: "name", Value: "two"},
				{Key: "features", Value: []any{
					sops.TreeBranch{{Key: "enabled", Value: true}},
				}},
			},
		}},
	}, branches[0])
}

func TestLoadPlainFilePreservesDottedAndQuotedKeys(t *testing.T) {
	branches, err := (&Store{}).LoadPlainFile([]byte(`"a.b"."c d" = 1
numeric.123 = "value"
empty."" = false
`))
	require.NoError(t, err)
	assert.Equal(t, sops.TreeBranch{
		{Key: "a.b", Value: sops.TreeBranch{{Key: "c d", Value: int64(1)}}},
		{Key: "numeric", Value: sops.TreeBranch{{Key: "123", Value: "value"}}},
		{Key: "empty", Value: sops.TreeBranch{{Key: "", Value: false}}},
	}, branches[0])
}

func TestLoadPlainFileHandlesIntegerBoundariesAndArrays(t *testing.T) {
	branches, err := (&Store{}).LoadPlainFile([]byte(`minimum = -9223372036854775808
maximum = 9223372036854775807
octal = 0o52
binary = 0b101010
underscored = 1_000_000
empty = []
nested = [[1, 2], [3, 4]]
tables = [{ name = "one" }, { name = "two" }]
`))
	require.NoError(t, err)
	assert.Equal(t, sops.TreeBranch{
		{Key: "minimum", Value: int64(math.MinInt64)},
		{Key: "maximum", Value: int64(math.MaxInt64)},
		{Key: "octal", Value: int64(42)},
		{Key: "binary", Value: int64(42)},
		{Key: "underscored", Value: int64(1_000_000)},
		{Key: "empty", Value: []any{}},
		{Key: "nested", Value: []any{
			[]any{int64(1), int64(2)},
			[]any{int64(3), int64(4)},
		}},
		{Key: "tables", Value: []any{
			sops.TreeBranch{{Key: "name", Value: "one"}},
			sops.TreeBranch{{Key: "name", Value: "two"}},
		}},
	}, branches[0])
}

func TestLoadPlainFileHandlesImplicitSuperTable(t *testing.T) {
	branches, err := (&Store{}).LoadPlainFile([]byte(`[parent.child]
value = 1
`))
	require.NoError(t, err)
	assert.Equal(t, sops.TreeBranch{
		{Key: "parent", Value: sops.TreeBranch{
			{Key: "child", Value: sops.TreeBranch{{Key: "value", Value: int64(1)}}},
		}},
	}, branches[0])
}

func TestLoadPlainFileHandlesEmptyAndCommentOnlyDocuments(t *testing.T) {
	branches, err := (&Store{}).LoadPlainFile(nil)
	require.NoError(t, err)
	assert.Equal(t, sops.TreeBranches{sops.TreeBranch{}}, branches)

	branches, err = (&Store{}).LoadPlainFile([]byte("# one\r\n#\r\n"))
	require.NoError(t, err)
	assert.Equal(t, sops.TreeBranches{sops.TreeBranch{
		{Key: sops.Comment{Value: "one"}},
		{Key: sops.Comment{Value: ""}},
	}}, branches)
}

func TestLoadPlainFileRejectsInvalidDocuments(t *testing.T) {
	tests := map[string][]byte{
		"invalid syntax":     []byte("value = ["),
		"duplicate key":      []byte("value = 1\nvalue = 2\n"),
		"conflicting tables": []byte("value = 1\n[value]\n"),
		"invalid UTF-8":      {'v', 'a', 'l', 'u', 'e', ' ', '=', ' ', '"', 0xff, '"'},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := (&Store{}).LoadPlainFile(input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "could not unmarshal TOML data")
		})
	}
}
