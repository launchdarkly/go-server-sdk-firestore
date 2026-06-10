package ldfirestore

import (
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
)

func TestDataStoreBuilder(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		b := DataStore("my-project", "my-collection")
		assert.Nil(t, b.client)
		assert.Equal(t, "", b.prefix)
		assert.Nil(t, b.clientOptions)
		assert.Equal(t, "my-project", b.projectID)
		assert.Equal(t, "my-collection", b.collection)
	})

	t.Run("Prefix", func(t *testing.T) {
		b := DataStore("my-project", "my-collection").Prefix("p")
		assert.Equal(t, "p", b.prefix)

		b.Prefix("")
		assert.Equal(t, "", b.prefix)
	})

	t.Run("FirestoreClient", func(t *testing.T) {
		// We can't actually create a client without a real connection, so we'll just verify
		// the builder accepts the parameter. The client would normally be created via
		// firestore.NewClient which requires authentication.
		var client *firestore.Client // nil is fine for this test
		b := DataStore("my-project", "my-collection").FirestoreClient(client)
		assert.Equal(t, client, b.client)
	})

	t.Run("ClientOptions", func(t *testing.T) {
		opt1 := option.WithEndpoint("localhost:8080")
		opt2 := option.WithoutAuthentication()

		b := DataStore("my-project", "my-collection").ClientOptions(opt1, opt2)
		assert.Len(t, b.clientOptions, 2)
	})

	t.Run("error for empty project ID", func(t *testing.T) {
		ds, err := DataStore("", "my-collection").Build(subsystems.BasicClientContext{})
		assert.Error(t, err)
		assert.Nil(t, ds)
		assert.Contains(t, err.Error(), "project ID is required")

		bs, err := BigSegmentStore("", "my-collection").Build(subsystems.BasicClientContext{})
		assert.Error(t, err)
		assert.Nil(t, bs)
		assert.Contains(t, err.Error(), "project ID is required")
	})

	t.Run("error for empty collection name", func(t *testing.T) {
		ds, err := DataStore("my-project", "").Build(subsystems.BasicClientContext{})
		assert.Error(t, err)
		assert.Nil(t, ds)
		assert.Contains(t, err.Error(), "collection name is required")

		bs, err := BigSegmentStore("my-project", "").Build(subsystems.BasicClientContext{})
		assert.Error(t, err)
		assert.Nil(t, bs)
		assert.Contains(t, err.Error(), "collection name is required")
	})

	t.Run("diagnostic description", func(t *testing.T) {
		value := DataStore("my-project", "my-collection").DescribeConfiguration()
		assert.Equal(t, ldvalue.String("Firestore"), value)
	})
}

func TestBigSegmentStoreBuilder(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		b := BigSegmentStore("my-project", "my-collection")
		assert.Nil(t, b.client)
		assert.Equal(t, "", b.prefix)
		assert.Nil(t, b.clientOptions)
		assert.Equal(t, "my-project", b.projectID)
		assert.Equal(t, "my-collection", b.collection)
	})

	t.Run("Prefix", func(t *testing.T) {
		b := BigSegmentStore("my-project", "my-collection").Prefix("p")
		assert.Equal(t, "p", b.prefix)
	})

	t.Run("diagnostic description", func(t *testing.T) {
		value := BigSegmentStore("my-project", "my-collection").DescribeConfiguration()
		assert.Equal(t, ldvalue.String("Firestore"), value)
	})
}
