package ldfirestore

import (
	"context"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/storetest"
)

func TestBigSegmentStore(t *testing.T) {
	if !isEmulatorAvailable() {
		t.Skip("Firestore emulator is not available. Set FIRESTORE_EMULATOR_HOST to run these tests.")
	}

	setTestMetadata := func(prefix string, metadata subsystems.BigSegmentStoreMetadata) error {
		client, err := createTestClient()
		if err != nil {
			return err
		}
		defer client.Close()

		ctx := context.Background()
		docID := makeTestDocID(prefix, bigSegmentsMetadataKey, bigSegmentsMetadataKey)
		docRef := client.Collection(testCollectionName).Doc(docID)

		data := map[string]any{
			fieldNamespace:          makeTestNamespace(prefix, bigSegmentsMetadataKey),
			fieldKey:                bigSegmentsMetadataKey,
			bigSegmentsSyncTimeAttr: int64(metadata.LastUpToDate),
		}

		_, err = docRef.Set(ctx, data)
		return err
	}

	setTestSegments := func(prefix string, contextHashKey string, included []string, excluded []string) error {
		client, err := createTestClient()
		if err != nil {
			return err
		}
		defer client.Close()

		ctx := context.Background()
		docID := makeTestDocID(prefix, bigSegmentsUserDataKey, contextHashKey)
		docRef := client.Collection(testCollectionName).Doc(docID)

		data := map[string]any{
			fieldNamespace: makeTestNamespace(prefix, bigSegmentsUserDataKey),
			fieldKey:       contextHashKey,
		}

		if len(included) > 0 {
			data[bigSegmentsIncludedAttr] = included
		}
		if len(excluded) > 0 {
			data[bigSegmentsExcludedAttr] = excluded
		}

		_, err = docRef.Set(ctx, data)
		return err
	}

	storetest.NewBigSegmentStoreTestSuite(
		func(prefix string) subsystems.ComponentConfigurer[subsystems.BigSegmentStore] {
			return baseBigSegmentStoreBuilder().Prefix(prefix)
		},
		clearTestData,
		setTestMetadata,
		setTestSegments,
	).Run(t)
}

func baseBigSegmentStoreBuilder() *StoreBuilder[subsystems.BigSegmentStore] {
	return BigSegmentStore(testProjectID, testCollectionName).ClientOptions(makeTestOptions()...)
}

// Helper functions for creating test document IDs and namespaces
func makeTestDocID(prefix, namespace, key string) string {
	fullNamespace := namespace
	if prefix != "" {
		fullNamespace = prefix + ":" + namespace
	}
	return fullNamespace + ":" + key
}

func makeTestNamespace(prefix, namespace string) string {
	if prefix == "" {
		return namespace
	}
	return prefix + ":" + namespace
}
