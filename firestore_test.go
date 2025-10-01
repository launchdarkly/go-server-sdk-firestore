package ldfirestore

import (
	"context"
	"os"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/storetest"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	testProjectID        = "test-project"
	testCollectionName   = "ld-test-collection"
	emulatorHost         = "FIRESTORE_EMULATOR_HOST"
	defaultEmulatorValue = "localhost:8080"
)

func TestFirestoreDataStore(t *testing.T) {
	if !isEmulatorAvailable() {
		t.Skip("Firestore emulator is not available. Set FIRESTORE_EMULATOR_HOST to run these tests.")
	}

	storetest.NewPersistentDataStoreTestSuite(makeTestStore, clearTestData).
		ErrorStoreFactory(makeFailedStore(), verifyFailedStoreError).
		ConcurrentModificationHook(setConcurrentModificationHook).
		Run(t)
}

func TestDataStoreSkipsAndLogsTooLargeItem(t *testing.T) {
	if !isEmulatorAvailable() {
		t.Skip("Firestore emulator is not available. Set FIRESTORE_EMULATOR_HOST to run these tests.")
	}

	makeGoodData := func() []ldstoretypes.SerializedCollection {
		return []ldstoretypes.SerializedCollection{
			{
				Kind: ldstoreimpl.Features(),
				Items: []ldstoretypes.KeyedSerializedItemDescriptor{
					{
						Key: "flag1",
						Item: ldstoretypes.SerializedItemDescriptor{
							Version: 1, SerializedItem: []byte(`{"key": "flag1", "version": 1}`),
						},
					},
					{
						Key: "flag2",
						Item: ldstoretypes.SerializedItemDescriptor{
							Version: 1, SerializedItem: []byte(`{"key": "flag2", "version": 1}`),
						},
					},
				},
			},
			{
				Kind: ldstoreimpl.Segments(),
				Items: []ldstoretypes.KeyedSerializedItemDescriptor{
					{
						Key: "segment1",
						Item: ldstoretypes.SerializedItemDescriptor{
							Version: 1, SerializedItem: []byte(`{"key": "segment1", "version": 1}`),
						},
					},
					{
						Key: "segment2",
						Item: ldstoretypes.SerializedItemDescriptor{
							Version:        1,
							SerializedItem: []byte(`{"key": "segment2", "version": 1}`),
						},
					},
				},
			},
		}
	}

	makeBigData := func() []byte {
		// Create data that exceeds our conservative 900KB limit
		bigString := make([]byte, 950000)
		for i := range bigString {
			bigString[i] = 'x'
		}
		return bigString
	}

	badItemKey := "baditem"
	tooBigFlag := ldbuilders.NewFlagBuilder(badItemKey).Version(1).
		AddRule(ldbuilders.NewRuleBuilder().Variation(0)).Build()
	tooBigFlagJSON := jsonhelpers.ToJSON(tooBigFlag)
	// Pad it to make it too big
	tooBigFlagJSON = append(tooBigFlagJSON, makeBigData()...)

	tooBigSegment := ldbuilders.NewSegmentBuilder(badItemKey).Version(1).Build()
	tooBigSegmentJSON := jsonhelpers.ToJSON(tooBigSegment)
	tooBigSegmentJSON = append(tooBigSegmentJSON, makeBigData()...)

	kindParams := []struct {
		name      string
		dataKind  ldstoretypes.DataKind
		collIndex int
		item      ldstoretypes.SerializedItemDescriptor
	}{
		{"flags", ldstoreimpl.Features(), 0, ldstoretypes.SerializedItemDescriptor{
			Version:        1,
			SerializedItem: tooBigFlagJSON,
		}},
		{"segments", ldstoreimpl.Segments(), 1, ldstoretypes.SerializedItemDescriptor{
			Version:        1,
			SerializedItem: tooBigSegmentJSON,
		}},
	}

	getAllData := func(t *testing.T, store subsystems.PersistentDataStore) []ldstoretypes.SerializedCollection {
		flags, err := store.GetAll(ldstoreimpl.Features())
		require.NoError(t, err)
		segments, err := store.GetAll(ldstoreimpl.Segments())
		require.NoError(t, err)
		return []ldstoretypes.SerializedCollection{
			{Kind: ldstoreimpl.Features(), Items: flags},
			{Kind: ldstoreimpl.Segments(), Items: segments},
		}
	}

	t.Run("init", func(t *testing.T) {
		for _, params := range kindParams {
			t.Run(params.name, func(t *testing.T) {
				mockLog := ldlogtest.NewMockLog()
				ctx := subsystems.BasicClientContext{}
				ctx.Logging.Loggers = mockLog.Loggers
				store, err := makeTestStore("").Build(ctx)
				require.NoError(t, err)
				defer store.Close()

				dataPlusBadItem := makeGoodData()
				collection := dataPlusBadItem[params.collIndex]
				collection.Items = append(
					// put the bad item first to prove that items after that one are still stored
					[]ldstoretypes.KeyedSerializedItemDescriptor{
						{Key: badItemKey, Item: params.item},
					},
					collection.Items...,
				)
				dataPlusBadItem[params.collIndex] = collection

				require.NoError(t, store.Init(dataPlusBadItem))

				mockLog.AssertMessageMatch(t, true, ldlog.Error, "was too large to store in Firestore and was dropped")

				assert.Equal(t, makeGoodData(), getAllData(t, store))
			})
		}
	})

	t.Run("upsert", func(t *testing.T) {
		for _, params := range kindParams {
			t.Run(params.name, func(t *testing.T) {
				mockLog := ldlogtest.NewMockLog()
				ctx := subsystems.BasicClientContext{}
				ctx.Logging.Loggers = mockLog.Loggers
				store, err := makeTestStore("").Build(ctx)
				require.NoError(t, err)
				defer store.Close()

				goodData := makeGoodData()
				require.NoError(t, store.Init(goodData))

				updated, err := store.Upsert(params.dataKind, badItemKey, params.item)
				assert.False(t, updated)
				assert.NoError(t, err)
				mockLog.AssertMessageMatch(t, true, ldlog.Error, "was too large to store in Firestore and was dropped")

				assert.Equal(t, goodData, getAllData(t, store))
			})
		}
	})
}

func baseDataStoreBuilder() *StoreBuilder[subsystems.PersistentDataStore] {
	return DataStore(testProjectID, testCollectionName).ClientOptions(makeTestOptions()...)
}

func makeTestStore(prefix string) subsystems.ComponentConfigurer[subsystems.PersistentDataStore] {
	return baseDataStoreBuilder().Prefix(prefix)
}

func makeFailedStore() subsystems.ComponentConfigurer[subsystems.PersistentDataStore] {
	// Use a closed client to ensure all operations error out.
	store := DataStore(testProjectID, testCollectionName)
	client, err := createTestClient()
	if err != nil {
		panic(err)
	}

	store.FirestoreClient(client)
	client.Close()

	return store
}

func verifyFailedStoreError(t assert.TestingT, err error) {
	// The exact error message may vary, but it should indicate a connection/auth failure
	assert.Error(t, err)
}

func clearTestData(prefix string) error {
	client, err := createTestClient()
	if err != nil {
		return err
	}
	defer client.Close()

	ctx := context.Background()
	coll := client.Collection(testCollectionName)

	// Delete all documents in the collection
	iter := coll.Documents(ctx)
	defer iter.Stop()

	// Use BulkWriter for efficient bulk deletes
	bulkWriter := client.BulkWriter(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}

		// Only delete documents with the matching prefix
		if prefix == "" || hasPrefix(doc.Ref.ID, prefix) {
			if _, err := bulkWriter.Delete(doc.Ref); err != nil {
				return err
			}
		}
	}

	// Flush all delete operations
	bulkWriter.End()

	return nil
}

func hasPrefix(docID, prefix string) bool {
	if prefix == "" {
		return true
	}
	// Document IDs are in format: {prefix}:{namespace}:{key}
	// We want to match documents that start with the prefix
	return len(docID) > len(prefix) && docID[:len(prefix)+1] == prefix+":"
}

func setConcurrentModificationHook(store subsystems.PersistentDataStore, hook func()) {
	store.(*firestoreDataStore).testUpdateHook = hook
}

func createTestClient() (*firestore.Client, error) {
	ctx := context.Background()
	return firestore.NewClient(ctx, testProjectID, makeTestOptions()...)
}

func makeTestOptions() []option.ClientOption {
	// When the emulator is running, we need these options
	emulatorAddr := os.Getenv(emulatorHost)
	if emulatorAddr == "" {
		emulatorAddr = defaultEmulatorValue
	}

	return []option.ClientOption{
		option.WithEndpoint(emulatorAddr),
		option.WithoutAuthentication(),
	}
}

func isEmulatorAvailable() bool {
	// Check if emulator is configured
	if os.Getenv(emulatorHost) == "" {
		// Try to set it to default and test
		os.Setenv(emulatorHost, defaultEmulatorValue)
	}

	// Try to create a client and do a simple operation
	client, err := createTestClient()
	if err != nil {
		return false
	}
	defer client.Close()

	// Try a simple operation to verify the emulator is responsive
	ctx := context.Background()
	_, err = client.Collection("test").Doc("test").Get(ctx)
	// We don't care if the document exists, just that we can connect
	return err == nil || err.Error() != "context deadline exceeded"
}
