package toml

import (
	"fmt"

	"github.com/YewFence/sops/v3"
	"github.com/YewFence/sops/v3/stores"
)

var _ sops.Store = (*Store)(nil)

// LoadEncryptedFile loads an encrypted TOML document and extracts its SOPS metadata.
func (store *Store) LoadEncryptedFile(in []byte) (sops.Tree, error) {
	branches, err := store.LoadPlainFile(in)
	if err != nil {
		return sops.Tree{}, err
	}
	branches, metadata, err := stores.ExtractMetadata(branches, stores.MetadataOpts{
		Flatten: stores.MetadataFlattenNone,
	})
	if err != nil {
		return sops.Tree{}, err
	}
	return sops.Tree{Branches: branches, Metadata: metadata}, nil
}

// EmitEncryptedFile serializes SOPS metadata after the encrypted user data.
func (store *Store) EmitEncryptedFile(tree sops.Tree) ([]byte, error) {
	branches, err := stores.SerializeMetadata(tree, stores.MetadataOpts{
		Flatten: stores.MetadataFlattenNone,
	})
	if err != nil {
		return nil, fmt.Errorf("error marshaling metadata: %w", err)
	}
	return store.EmitPlainFile(branches)
}

// EmitExample returns a representative plaintext TOML document.
func (store *Store) EmitExample() []byte {
	out, err := store.EmitPlainFile(stores.ExampleComplexTree.Branches)
	if err != nil {
		panic(err)
	}
	return out
}

// HasSopsTopLevelKey reports whether a branch contains the reserved sops key.
func (store *Store) HasSopsTopLevelKey(branch sops.TreeBranch) bool {
	return stores.HasSopsTopLevelKey(branch)
}
