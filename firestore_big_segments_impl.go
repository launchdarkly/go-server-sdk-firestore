package ldfirestore

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	bigSegmentsMetadataKey  = "big_segments_metadata"
	bigSegmentsUserDataKey  = "big_segments_user"
	bigSegmentsSyncTimeAttr = "synchronizedOn"
	bigSegmentsIncludedAttr = "included"
	bigSegmentsExcludedAttr = "excluded"
)

// Internal implementation of the BigSegmentStore interface for Firestore.
type firestoreBigSegmentStoreImpl struct {
	client        *firestore.Client
	context       context.Context
	cancelContext func()
	collection    string
	prefix        string
	loggers       ldlog.Loggers
	ownsClient    bool // true if we created the client and should close it
}

func newFirestoreBigSegmentStoreImpl(
	builder builderOptions,
	loggers ldlog.Loggers,
) (*firestoreBigSegmentStoreImpl, error) {
	if builder.collection == "" {
		return nil, errors.New("collection name is required")
	}

	client := builder.client
	ctx, cancelContext := context.WithCancel(context.Background())
	ownsClient := false

	// If a client was provided, use it directly. Otherwise, create a new one.
	// We only close clients that we create ourselves.
	if client == nil {
		var err error
		if client, ctx, cancelContext, err = makeClientAndContext(builder); err != nil {
			return nil, err
		}
		ownsClient = true
	}

	store := &firestoreBigSegmentStoreImpl{
		client:        client,
		context:       ctx,
		cancelContext: cancelContext,
		collection:    builder.collection,
		prefix:        builder.prefix,
		loggers:       loggers, // copied by value so we can modify it
		ownsClient:    ownsClient,
	}
	store.loggers.SetPrefix("FirestoreBigSegmentStore:")
	store.loggers.Infof(`Using Firestore collection %s`, store.collection)

	return store, nil
}

func (store *firestoreBigSegmentStoreImpl) GetMetadata() (subsystems.BigSegmentStoreMetadata, error) {
	docID := store.makeDocID(bigSegmentsMetadataKey, bigSegmentsMetadataKey)
	docRef := store.client.Collection(store.collection).Doc(docID)

	doc, err := docRef.Get(store.context)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// this is just a "not found" result, not a database error
			return subsystems.BigSegmentStoreMetadata{}, nil
		}
		return subsystems.BigSegmentStoreMetadata{}, err
	}

	if !doc.Exists() {
		return subsystems.BigSegmentStoreMetadata{}, nil
	}

	data := doc.Data()
	value, ok := data[bigSegmentsSyncTimeAttr].(int64)
	if !ok || value == 0 {
		return subsystems.BigSegmentStoreMetadata{}, nil
	}

	return subsystems.BigSegmentStoreMetadata{
		LastUpToDate: ldtime.UnixMillisecondTime(uint64(value)),
	}, nil
}

func (store *firestoreBigSegmentStoreImpl) GetMembership(
	contextHashKey string,
) (subsystems.BigSegmentMembership, error) {
	docID := store.makeDocID(bigSegmentsUserDataKey, contextHashKey)
	docRef := store.client.Collection(store.collection).Doc(docID)

	doc, err := docRef.Get(store.context)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(nil, nil), nil
		}
		return nil, err
	}

	if !doc.Exists() {
		return ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(nil, nil), nil
	}

	data := doc.Data()
	includedRefs, err := getStringSliceFromInterface(data, bigSegmentsIncludedAttr)
	if err != nil {
		return nil, err
	}
	excludedRefs, err := getStringSliceFromInterface(data, bigSegmentsExcludedAttr)
	if err != nil {
		return nil, err
	}

	return ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(includedRefs, excludedRefs), nil
}

func getStringSliceFromInterface(data map[string]any, key string) ([]string, error) {
	value, found := data[key]
	if !found {
		return nil, nil // attribute is optional
	}

	if arr, ok := value.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, v := range arr {
			if str, ok := v.(string); ok {
				result = append(result, str)
			} else {
				return nil, fmt.Errorf("expected string array but found %v", v)
			}
		}
		return result, nil
	}

	return nil, errors.New("expected string array")
}

func (store *firestoreBigSegmentStoreImpl) Close() error {
	store.cancelContext() // stops any pending operations
	// Only close the client if we created it. If a client was provided to us,
	// it's the caller's responsibility to close it.
	if store.ownsClient {
		return store.client.Close()
	}
	return nil
}

func (store *firestoreBigSegmentStoreImpl) makeDocID(namespace, key string) string {
	// Document ID format: {prefix}:{namespace}:{key}
	fullNamespace := namespace
	if store.prefix != "" {
		fullNamespace = store.prefix + ":" + namespace
	}
	return fullNamespace + ":" + key
}
