package common

import (
	"testing"

	"github.com/YewFence/sops/v3/cmd/sops/formats"
	"github.com/YewFence/sops/v3/config"
	"github.com/stretchr/testify/assert"
)

func TestTOMLStoreSelection(t *testing.T) {
	storesConfig := config.NewStoresConfig()
	tests := []struct {
		name  string
		store Store
	}{
		{name: "format enum", store: StoreForFormat(formats.Toml, storesConfig)},
		{name: "extension", store: DefaultStoreForPath(storesConfig, "secrets.toml")},
		{name: "explicit format", store: DefaultStoreForPathOrFormat(storesConfig, "secrets", "toml")},
		{name: "explicit override", store: DefaultStoreForPathOrFormat(storesConfig, "secrets.yaml", "toml")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, "toml", test.store.Name())
		})
	}
}
