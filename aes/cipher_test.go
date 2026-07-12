package aes

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/getsops/sops/v3"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
)

func TestDecrypt(t *testing.T) {
	expected := "foo"
	key := []byte(strings.Repeat("f", 32))
	message := `ENC[AES256_GCM,data:oYyi,iv:MyIDYbT718JRr11QtBkcj3Dwm4k1aCGZBVeZf0EyV8o=,tag:t5z2Z023Up0kxwCgw1gNxg==,type:str]`
	decryption, err := NewCipher().Decrypt(message, key, "bar:")
	if err != nil {
		t.Errorf("%s", err)
	}
	if decryption != expected {
		t.Errorf("Decrypt(\"%s\", \"%s\") == \"%s\", expected %s", message, key, decryption, expected)
	}
}

func TestDecryptInvalidAad(t *testing.T) {
	message := `ENC[AES256_GCM,data:oYyi,iv:MyIDYbT718JRr11QtBkcj3Dwm4k1aCGZBVeZf0EyV8o=,tag:t5z2Z023Up0kxwCgw1gNxg==,type:str]`
	_, err := NewCipher().Decrypt(message, []byte(strings.Repeat("f", 32)), "")
	if err == nil {
		t.Errorf("Decrypting with an invalid AAC should fail")
	}
}

func TestRoundtripString(t *testing.T) {
	f := func(x, aad string) bool {
		key := make([]byte, 32)
		rand.Read(key)
		s, err := NewCipher().Encrypt(x, key, aad)
		if err != nil {
			log.Println(err)
			return false
		}
		d, err := NewCipher().Decrypt(s, key, aad)
		if err != nil {
			return false
		}
		return x == d
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestRoundtripFloat(t *testing.T) {
	key := []byte(strings.Repeat("f", 32))
	f := func(x float64) bool {
		s, err := NewCipher().Encrypt(x, key, "")
		if err != nil {
			log.Println(err)
			return false
		}
		d, err := NewCipher().Decrypt(s, key, "")
		if err != nil {
			return false
		}
		return x == d
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestRoundtripInt(t *testing.T) {
	key := []byte(strings.Repeat("f", 32))
	f := func(x int) bool {
		s, err := NewCipher().Encrypt(x, key, "")
		if err != nil {
			log.Println(err)
			return false
		}
		d, err := NewCipher().Decrypt(s, key, "")
		if err != nil {
			return false
		}
		return x == d
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestRoundtripBool(t *testing.T) {
	key := []byte(strings.Repeat("f", 32))
	f := func(x bool) bool {
		s, err := NewCipher().Encrypt(x, key, "")
		if err != nil {
			log.Println(err)
			return false
		}
		d, err := NewCipher().Decrypt(s, key, "")
		if err != nil {
			return false
		}
		return x == d
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestRoundtripTime(t *testing.T) {
	key := []byte(strings.Repeat("f", 32))
	parsedTime, err := time.Parse(time.RFC3339, "2006-01-02T15:04:05+07:00")
	assert.Nil(t, err)
	loc := time.FixedZone("", 12300) // offset must be divisible by 60, otherwise won't survive a round-trip
	values := []time.Time{
		time.UnixMilli(0).In(time.UTC),
		time.UnixMilli(123456).In(time.UTC),
		time.UnixMilli(123456).In(loc),
		time.UnixMilli(123456789).In(time.UTC),
		time.UnixMilli(123456789).In(loc),
		time.UnixMilli(1234567890).In(time.UTC),
		time.UnixMilli(1234567890).In(loc),
		parsedTime,
	}
	for _, value := range values {
		s, err := NewCipher().Encrypt(value, key, "foo")
		assert.Nil(t, err)
		if err != nil {
			continue
		}
		d, err := NewCipher().Decrypt(s, key, "foo")
		assert.Nil(t, err)
		if err != nil {
			continue
		}
		assert.Equal(t, value, d)
	}
}

func TestRoundtripTOMLScalarTypes(t *testing.T) {
	var localDateTime toml.LocalDateTime
	assert.NoError(t, localDateTime.UnmarshalText([]byte("1979-05-27T07:32:00")))
	var localDate toml.LocalDate
	assert.NoError(t, localDate.UnmarshalText([]byte("1979-05-27")))
	var localTime toml.LocalTime
	assert.NoError(t, localTime.UnmarshalText([]byte("07:32:00")))
	values := []interface{}{
		int64(-9223372036854775808),
		int64(9223372036854775807),
		localDateTime,
		localDate,
		localTime,
	}
	key := []byte(strings.Repeat("f", 32))
	for _, value := range values {
		cipher := NewCipher()
		encrypted, err := cipher.Encrypt(value, key, "toml")
		assert.NoError(t, err)
		decrypted, err := cipher.Decrypt(encrypted, key, "toml")
		assert.NoError(t, err)
		assert.Equal(t, value, decrypted)
	}
	expected := sops.TreeBranch{}
	for i, value := range values {
		expected = append(expected, sops.TreeItem{Key: fmt.Sprintf("value_%d", i), Value: value})
	}
	tree := sops.Tree{Branches: sops.TreeBranches{append(sops.TreeBranch{}, expected...)}}
	cipher := NewCipher()
	_, err := tree.Encrypt(key, cipher)
	assert.NoError(t, err)
	_, err = tree.Decrypt(key, cipher)
	assert.NoError(t, err)
	assert.Equal(t, expected, tree.Branches[0])
}

func TestEncryptEmptyComment(t *testing.T) {
	key := []byte(strings.Repeat("f", 32))
	s, err := NewCipher().Encrypt(sops.Comment{}, key, "")
	assert.Nil(t, err)
	assert.Equal(t, "", s)
}

func TestRoundtripInlineComment(t *testing.T) {
	key := []byte(strings.Repeat("f", 32))
	expected := sops.Comment{Value: "inline", Inline: true}
	cipher := NewCipher()
	encrypted, err := cipher.Encrypt(expected, key, "comment")
	assert.NoError(t, err)
	parsed, err := parse(encrypted)
	assert.NoError(t, err)
	assert.Equal(t, "comment_inline", parsed.datatype)
	decrypted, err := cipher.Decrypt(encrypted, key, "comment")
	assert.NoError(t, err)
	assert.Equal(t, expected, decrypted)

	tree := sops.Tree{Branches: sops.TreeBranches{sops.TreeBranch{
		{Key: "values", Value: []interface{}{expected, "value"}},
	}}}
	_, err = tree.Encrypt(key, cipher)
	assert.NoError(t, err)
	assert.IsType(t, "", tree.Branches[0][0].Value.([]interface{})[0])
	_, err = tree.Decrypt(key, cipher)
	assert.NoError(t, err)
	assert.Equal(t, expected, tree.Branches[0][0].Value.([]interface{})[0])
}

func TestDecryptEmptyValue(t *testing.T) {
	key := []byte(strings.Repeat("f", 32))
	s, err := NewCipher().Decrypt("", key, "")
	assert.Nil(t, err)
	assert.Equal(t, "", s)
}

// This test would belong more in sops_test.go, but from there we cannot access
// the aes package to get a cipher which can actually handle time.Time objects.
func TestTimestamps(t *testing.T) {
	unixTime := time.UnixMilli(123456789).In(time.UTC)
	parsedTime, err := time.Parse(time.RFC3339, "2006-01-02T15:04:05+07:00")
	assert.Nil(t, err)
	branches := sops.TreeBranches{
		sops.TreeBranch{
			sops.TreeItem{
				Key:   "foo",
				Value: unixTime,
			},
			sops.TreeItem{
				Key: "bar",
				Value: sops.TreeBranch{
					sops.TreeItem{
						Key:   "foo",
						Value: parsedTime,
					},
				},
			},
		},
	}
	tree := sops.Tree{Branches: branches, Metadata: sops.Metadata{UnencryptedSuffix: "_unencrypted"}}
	expected := sops.TreeBranch{
		sops.TreeItem{
			Key:   "foo",
			Value: unixTime,
		},
		sops.TreeItem{
			Key: "bar",
			Value: sops.TreeBranch{
				sops.TreeItem{
					Key:   "foo",
					Value: parsedTime,
				},
			},
		},
	}
	cipher := NewCipher()
	_, err = tree.Encrypt(bytes.Repeat([]byte("f"), 32), cipher)
	if err != nil {
		t.Errorf("Encrypting the tree failed: %s", err)
	}
	if reflect.DeepEqual(tree.Branches[0], expected) {
		t.Errorf("Trees do match: \ngot \t\t%+v,\n not expected \t\t%+v", tree.Branches[0], expected)
	}
	_, err = tree.Decrypt(bytes.Repeat([]byte("f"), 32), cipher)
	if err != nil {
		t.Errorf("Decrypting the tree failed: %s", err)
	}
	assert.Equal(t, tree.Branches[0][0].Value, unixTime)
	assert.Equal(t, tree.Branches[0], expected)
	if !reflect.DeepEqual(tree.Branches[0], expected) {
		t.Errorf("Trees don't match: \ngot\t\t\t%+v,\nexpected\t\t%+v", tree.Branches[0], expected)
	}
}
