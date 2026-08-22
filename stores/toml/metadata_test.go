package toml

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/YewFence/sops/v3"
	"github.com/YewFence/sops/v3/aes"
	"github.com/YewFence/sops/v3/age"
	"github.com/YewFence/sops/v3/azkv"
	"github.com/YewFence/sops/v3/config"
	"github.com/YewFence/sops/v3/gcpkms"
	"github.com/YewFence/sops/v3/hckms"
	"github.com/YewFence/sops/v3/hcvault"
	"github.com/YewFence/sops/v3/kms"
	"github.com/YewFence/sops/v3/pgp"
	"github.com/YewFence/sops/v3/stores"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func completeMetadata() sops.Metadata {
	createdAt := time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
	tenant := "example-tenant"
	return sops.Metadata{
		ShamirThreshold:           2,
		LastModified:              createdAt,
		MessageAuthenticationCode: "encrypted-mac",
		EncryptedCommentRegex:     "encrypt me",
		MACOnlyEncrypted:          true,
		Version:                   "3.13.2",
		KeyGroups: []sops.KeyGroup{
			{
				&kms.MasterKey{
					Arn:          "arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000",
					Role:         "arn:aws:iam::000000000000:role/sops",
					EncryptedKey: "aws-encrypted-key",
					CreationDate: createdAt,
					EncryptionContext: map[string]*string{
						"tenant": &tenant,
					},
					AwsProfile: "production",
				},
				&gcpkms.MasterKey{
					ResourceID:   "projects/project/locations/global/keyRings/ring/cryptoKeys/key",
					EncryptedKey: "gcp-encrypted-key",
					CreationDate: createdAt,
				},
				&hckms.MasterKey{
					KeyID:        "eu-west-101:key-id",
					Region:       "eu-west-101",
					KeyUUID:      "key-id",
					EncryptedKey: "huawei-encrypted-key",
					CreationDate: createdAt,
				},
				&azkv.MasterKey{
					VaultURL:     "https://vault.vault.azure.net",
					Name:         "key-name",
					Version:      "key-version",
					EncryptedKey: "azure-encrypted-key",
					CreationDate: createdAt,
				},
				&hcvault.MasterKey{
					VaultAddress: "https://vault.example.com",
					EnginePath:   "transit",
					KeyName:      "sops",
					EncryptedKey: "vault-encrypted-key",
					CreationDate: createdAt,
				},
				&pgp.MasterKey{
					Fingerprint:  "0123456789ABCDEF0123456789ABCDEF01234567",
					EncryptedKey: "pgp-encrypted-key",
					CreationDate: createdAt,
				},
				&age.MasterKey{
					Recipient:    "age1example",
					EncryptedKey: "age-encrypted-key",
				},
			},
			{
				&pgp.MasterKey{
					Fingerprint:  "89ABCDEF0123456789ABCDEF0123456789ABCDEF",
					EncryptedKey: "second-pgp-encrypted-key",
					CreationDate: createdAt,
				},
			},
		},
	}
}

func TestEncryptedMetadataRoundTrip(t *testing.T) {
	store := &Store{}
	tree := sops.Tree{
		Branches: sops.TreeBranches{sops.TreeBranch{
			{Key: "secret", Value: "ENC[example]"},
		}},
		Metadata: completeMetadata(),
	}
	first, err := store.EmitEncryptedFile(tree)
	require.NoError(t, err)
	assert.Contains(t, string(first), "[sops]")
	assert.Contains(t, string(first), "[[sops.key_groups]]")
	assert.Contains(t, string(first), "[[sops.key_groups.kms]]")
	assert.Contains(t, string(first), "[sops.key_groups.kms.context]")
	loaded, err := store.LoadEncryptedFile(first)
	require.NoError(t, err)
	assert.Equal(t, tree.Branches, loaded.Branches)
	assert.Equal(t, tree.Metadata, loaded.Metadata)
	second, err := store.EmitEncryptedFile(loaded)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

func TestMetadataSelectionSettingsRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		set    func(*sops.Metadata)
		assert func(*testing.T, sops.Metadata)
	}{
		{
			name: "unencrypted suffix",
			set:  func(metadata *sops.Metadata) { metadata.UnencryptedSuffix = "_plain" },
			assert: func(t *testing.T, metadata sops.Metadata) {
				assert.Equal(t, "_plain", metadata.UnencryptedSuffix)
			},
		},
		{
			name: "encrypted suffix",
			set:  func(metadata *sops.Metadata) { metadata.EncryptedSuffix = "_secret" },
			assert: func(t *testing.T, metadata sops.Metadata) {
				assert.Equal(t, "_secret", metadata.EncryptedSuffix)
			},
		},
		{
			name: "unencrypted regex",
			set:  func(metadata *sops.Metadata) { metadata.UnencryptedRegex = "^public$" },
			assert: func(t *testing.T, metadata sops.Metadata) {
				assert.Equal(t, "^public$", metadata.UnencryptedRegex)
			},
		},
		{
			name: "encrypted regex",
			set:  func(metadata *sops.Metadata) { metadata.EncryptedRegex = "^secret$" },
			assert: func(t *testing.T, metadata sops.Metadata) {
				assert.Equal(t, "^secret$", metadata.EncryptedRegex)
			},
		},
		{
			name: "unencrypted comment regex",
			set:  func(metadata *sops.Metadata) { metadata.UnencryptedCommentRegex = "keep" },
			assert: func(t *testing.T, metadata sops.Metadata) {
				assert.Equal(t, "keep", metadata.UnencryptedCommentRegex)
			},
		},
		{
			name: "encrypted comment regex",
			set:  func(metadata *sops.Metadata) { metadata.EncryptedCommentRegex = "encrypt" },
			assert: func(t *testing.T, metadata sops.Metadata) {
				assert.Equal(t, "encrypt", metadata.EncryptedCommentRegex)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := completeMetadata()
			metadata.EncryptedCommentRegex = ""
			metadata.KeyGroups = metadata.KeyGroups[:1]
			metadata.ShamirThreshold = 0
			test.set(&metadata)
			tree := sops.Tree{Branches: sops.TreeBranches{sops.TreeBranch{{Key: "value", Value: "ENC[example]"}}}, Metadata: metadata}
			out, err := (&Store{}).EmitEncryptedFile(tree)
			require.NoError(t, err)
			loaded, err := (&Store{}).LoadEncryptedFile(out)
			require.NoError(t, err)
			test.assert(t, loaded.Metadata)
			assert.True(t, loaded.Metadata.MACOnlyEncrypted)
		})
	}
}

func TestRealCipherRoundTripPreservesTOMLTypesAndComments(t *testing.T) {
	store := &Store{}
	plaintext := []byte(`# keep this comment
integer = 9223372036854775807 # integer inline
offset = 1979-05-27T07:32:00-07:00
local_datetime = 1979-05-27T07:32:00
local_date = 1979-05-27
local_time = 07:32:00
plain_unencrypted = "visible"
values = [1, # array inline
  2]
`)
	expected, err := store.LoadPlainFile(plaintext)
	require.NoError(t, err)
	branches, err := store.LoadPlainFile(plaintext)
	require.NoError(t, err)
	tree := sops.Tree{
		Branches: branches,
		Metadata: sops.Metadata{
			UnencryptedSuffix: "_unencrypted",
			MACOnlyEncrypted:  true,
		},
	}
	key := bytes.Repeat([]byte("k"), 32)
	cipher := aes.NewCipher()
	encryptedMAC, err := tree.Encrypt(key, cipher)
	require.NoError(t, err)
	encryptedFile, err := store.EmitPlainFile(tree.Branches)
	require.NoError(t, err)
	assert.Contains(t, string(encryptedFile), "plain_unencrypted = 'visible'")
	assert.Contains(t, string(encryptedFile), "type:int64")
	assert.Contains(t, string(encryptedFile), "type:toml_local_datetime")
	assert.Contains(t, string(encryptedFile), "type:comment_inline")
	loadedBranches, err := store.LoadPlainFile(encryptedFile)
	require.NoError(t, err)
	loaded := sops.Tree{Branches: loadedBranches, Metadata: tree.Metadata}
	decryptedMAC, err := loaded.Decrypt(key, cipher)
	require.NoError(t, err)
	assert.Equal(t, encryptedMAC, decryptedMAC)
	assert.Equal(t, expected, loaded.Branches)
}

func TestRealCipherRoundTripDecryptsExpandedInlineTableComment(t *testing.T) {
	store := &Store{}
	branches, err := store.LoadPlainFile([]byte(`inline = { z = "first", a = "second" } # inline table comment
`))
	require.NoError(t, err)
	tree := sops.Tree{Branches: branches}
	key := bytes.Repeat([]byte("k"), 32)
	cipher := aes.NewCipher()
	_, err = tree.Encrypt(key, cipher)
	require.NoError(t, err)
	encryptedFile, err := store.EmitPlainFile(tree.Branches)
	require.NoError(t, err)
	assert.Contains(t, string(encryptedFile), "[inline] # ENC[")
	loadedBranches, err := store.LoadPlainFile(encryptedFile)
	require.NoError(t, err)
	loaded := sops.Tree{Branches: loadedBranches}
	_, err = loaded.Decrypt(key, cipher)
	require.NoError(t, err)
	decryptedFile, err := store.EmitPlainFile(loaded.Branches)
	require.NoError(t, err)
	assert.NotContains(t, string(decryptedFile), "ENC[")
	assert.Contains(t, string(decryptedFile), "[inline] # inline table comment")
}

func TestEncryptedFileErrorsAndReservedKey(t *testing.T) {
	store := &Store{}
	_, err := store.LoadEncryptedFile([]byte("value = 1\n"))
	assert.ErrorIs(t, err, sops.MetadataNotFound)
	_, err = store.LoadEncryptedFile([]byte("sops = 'not a table'\n"))
	require.EqualError(t, err, "Found sops entry that is not a mapping")
	_, err = store.EmitEncryptedFile(sops.Tree{
		Branches: sops.TreeBranches{sops.TreeBranch{{Key: stores.SopsMetadataKey, Value: sops.TreeBranch{}}}},
		Metadata: completeMetadata(),
	})
	assert.ErrorContains(t, err, `Found key "sops" in encrypted data`)
	assert.True(t, store.HasSopsTopLevelKey(sops.TreeBranch{{Key: "sops", Value: sops.TreeBranch{}}}))
	assert.False(t, store.HasSopsTopLevelKey(sops.TreeBranch{{Key: "nested", Value: sops.TreeBranch{{Key: "sops"}}}}))
}

func TestNameAndExample(t *testing.T) {
	store := NewStore(&configForTest)
	assert.Equal(t, "toml", store.Name())
	example := store.EmitExample()
	assert.NotEmpty(t, example)
	_, err := store.LoadPlainFile(example)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(example), "hello ="))
}

var configForTest = config.TOMLStoreConfig{}
