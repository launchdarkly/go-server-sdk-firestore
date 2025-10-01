package ldfirestore

// Implementation notes:
//
// - Feature flags, segments, and any other kind of entity the LaunchDarkly client may wish
// to store, are all put in the same collection. The document ID is constructed as
// "{prefix}:{namespace}:{key}" where namespace disambiguates between flags and segments.
//
// - The entire object is serialized to JSON and stored in the "item" field. The "version"
// field is also stored separately since it is used for conditional updates. The "namespace"
// and "key" fields are stored to facilitate querying.
//
// - The Init method uses BulkWriter to write operations in batches. BulkWriter automatically
// batches operations (up to 20 per batch) and sends them in parallel for performance. However,
// BulkWriter does NOT provide atomicity guarantees - partial failures can occur. If another
// process is adding data via Upsert during Init, there can be race conditions. To minimize
// issues, we don't delete all the data at the start; instead, we update the items we've
// received, and then delete all other items. That could potentially result in deleting new
// data from another process, but that would be the case anyway if the Init happened to
// execute later than the Upsert; we are relying on the fact that normally the process that
// did the Init will also receive the new data shortly and do its own Upsert.
//
// - Firestore has a maximum document size of 1 MiB. Since each feature flag or user segment is
// stored as a single document, this mechanism will not work for extremely large flags or segments.

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Document field names
	fieldNamespace = "namespace"
	fieldKey       = "key"
	fieldVersion   = "version"
	fieldItem      = "item"

	// We won't try to store items whose total size exceeds this. Firestore's actual limit
	// is 1 MiB, but we use a conservative limit to account for field overhead and indexing.
	firestoreMaxDocSize = 900000 // ~900 KB
)

// Internal type for our Firestore implementation of the PersistentDataStore interface.
type firestoreDataStore struct {
	client         *firestore.Client
	context        context.Context
	cancelContext  func()
	collection     string
	prefix         string
	loggers        ldlog.Loggers
	testUpdateHook func() // Used only by unit tests
}

func newFirestoreDataStoreImpl(builder builderOptions, loggers ldlog.Loggers) (*firestoreDataStore, error) {
	if builder.collection == "" {
		return nil, errors.New("collection name is required")
	}

	client, ctx, cancelContext, err := makeClientAndContext(builder)
	if err != nil {
		return nil, err
	}

	store := &firestoreDataStore{
		client:        client,
		context:       ctx,
		cancelContext: cancelContext,
		collection:    builder.collection,
		prefix:        builder.prefix,
		loggers:       loggers, // copied by value so we can modify it
	}
	store.loggers.SetPrefix("ldfirestore:")
	store.loggers.Infof(`Using Firestore collection %s`, store.collection)

	return store, nil
}

func (store *firestoreDataStore) Init(allData []ldstoretypes.SerializedCollection) error {
	// Start by reading the existing document IDs; we will later delete any of these that weren't in allData.
	unusedOldIDs, err := store.readExistingDocIDs(allData)
	if err != nil {
		return fmt.Errorf("failed to get existing items prior to Init: %w", err)
	}

	operations := make([]firestoreOperation, 0)
	numItems := 0

	// Insert or update every provided item
	for _, coll := range allData {
		for _, item := range coll.Items {
			docID := store.makeDocID(coll.Kind, item.Key)
			docRef := store.client.Collection(store.collection).Doc(docID)

			data := store.encodeItem(coll.Kind, item.Key, item.Item)
			if !store.checkSizeLimit(data) {
				continue
			}

			operations = append(operations, setOperation{
				ref:  docRef,
				data: data,
			})
			unusedOldIDs[docID] = false
			numItems++
		}
	}

	// Now delete any previously existing items whose keys were not in the current data
	initedKey := store.initedDocID()
	for docID, shouldDelete := range unusedOldIDs {
		if shouldDelete && docID != initedKey {
			docRef := store.client.Collection(store.collection).Doc(docID)
			operations = append(operations, deleteOperation{ref: docRef})
		}
	}

	// Now set the special key that we check in IsInitialized()
	initedDocRef := store.client.Collection(store.collection).Doc(initedKey)
	operations = append(operations, setOperation{
		ref: initedDocRef,
		data: map[string]any{
			fieldNamespace: store.initedKey(),
			fieldKey:       store.initedKey(),
		},
	})

	if err := batchWriteOperations(store.context, store.client, operations); err != nil {
		return fmt.Errorf("failed to write %d item(s) in batches: %w", len(operations), err)
	}

	store.loggers.Infof("Initialized collection %q with %d item(s)", store.collection, numItems)

	return nil
}

func (store *firestoreDataStore) IsInitialized() bool {
	docRef := store.client.Collection(store.collection).Doc(store.initedDocID())
	_, err := docRef.Get(store.context)
	return err == nil
}

func (store *firestoreDataStore) GetAll(
	kind ldstoretypes.DataKind,
) ([]ldstoretypes.KeyedSerializedItemDescriptor, error) {
	namespace := store.namespaceForKind(kind)
	query := store.client.Collection(store.collection).Where(fieldNamespace, "==", namespace)

	iter := query.Documents(store.context)
	defer iter.Stop()

	var results []ldstoretypes.KeyedSerializedItemDescriptor
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate documents: %w", err)
		}

		key, serializedItemDesc, ok := store.decodeDocument(doc)
		if ok {
			results = append(results, ldstoretypes.KeyedSerializedItemDescriptor{
				Key:  key,
				Item: serializedItemDesc,
			})
		}
	}

	return results, nil
}

func (store *firestoreDataStore) Get(
	kind ldstoretypes.DataKind,
	key string,
) (ldstoretypes.SerializedItemDescriptor, error) {
	docID := store.makeDocID(kind, key)
	docRef := store.client.Collection(store.collection).Doc(docID)

	doc, err := docRef.Get(store.context)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			if store.loggers.IsDebugEnabled() {
				store.loggers.Debugf("Item not found (key=%s)", key)
			}
			return ldstoretypes.SerializedItemDescriptor{}.NotFound(), nil
		}
		return ldstoretypes.SerializedItemDescriptor{}.NotFound(),
			fmt.Errorf("failed to get %s key %s: %w", kind, key, err)
	}

	if !doc.Exists() {
		if store.loggers.IsDebugEnabled() {
			store.loggers.Debugf("Item not found (key=%s)", key)
		}
		return ldstoretypes.SerializedItemDescriptor{}.NotFound(), nil
	}

	if _, serializedItemDesc, ok := store.decodeDocument(doc); ok {
		return serializedItemDesc, nil
	}

	return ldstoretypes.SerializedItemDescriptor{}.NotFound(),
		fmt.Errorf("invalid data for %s key %s", kind, key)
}

func (store *firestoreDataStore) Upsert(
	kind ldstoretypes.DataKind,
	key string,
	newItem ldstoretypes.SerializedItemDescriptor,
) (bool, error) {
	data := store.encodeItem(kind, key, newItem)
	if !store.checkSizeLimit(data) {
		return false, nil
	}

	if store.testUpdateHook != nil {
		store.testUpdateHook()
	}

	docID := store.makeDocID(kind, key)
	docRef := store.client.Collection(store.collection).Doc(docID)

	// Use a transaction to ensure version checking
	err := store.client.RunTransaction(store.context, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(docRef)

		var oldVersion int
		if err == nil {
			if doc.Exists() {
				if v, ok := doc.Data()[fieldVersion].(int64); ok {
					oldVersion = int(v)
				}
			}
		} else if status.Code(err) == codes.NotFound {
			oldVersion = -1
		} else {
			// Any error other than NotFound is a real error
			return err
		}

		if oldVersion >= newItem.Version {
			if store.loggers.IsDebugEnabled() {
				store.loggers.Debugf("Not updating item due to version check (namespace=%s key=%s version=%d, existing=%d)",
					kind, key, newItem.Version, oldVersion)
			}
			return errVersionCheckFailed
		}

		return tx.Set(docRef, data)
	})

	if err == errVersionCheckFailed {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to upsert %s key %s: %w", kind, key, err)
	}

	return true, nil
}

var errVersionCheckFailed = errors.New("version check failed")

func (store *firestoreDataStore) IsStoreAvailable() bool {
	// Test the connection by trying to get the inited document
	docRef := store.client.Collection(store.collection).Doc(store.initedDocID())
	_, err := docRef.Get(store.context)
	// Both "found" and "not found" are acceptable - we just want to know the connection works
	return err == nil
}

func (store *firestoreDataStore) Close() error {
	store.cancelContext() // stops any pending operations
	return store.client.Close()
}

func (store *firestoreDataStore) prefixedNamespace(baseNamespace string) string {
	if store.prefix == "" {
		return baseNamespace
	}
	return store.prefix + ":" + baseNamespace
}

func (store *firestoreDataStore) namespaceForKind(kind ldstoretypes.DataKind) string {
	return store.prefixedNamespace(kind.GetName())
}

func (store *firestoreDataStore) initedKey() string {
	return store.prefixedNamespace("$inited")
}

func (store *firestoreDataStore) initedDocID() string {
	return store.makeDocIDFromParts(store.initedKey(), store.initedKey())
}

func (store *firestoreDataStore) makeDocID(kind ldstoretypes.DataKind, key string) string {
	return store.makeDocIDFromParts(store.namespaceForKind(kind), key)
}

func (store *firestoreDataStore) makeDocIDFromParts(namespace, key string) string {
	// Document ID format: {prefix}:{namespace}:{key}
	// Colons are allowed in Firestore document IDs
	if store.prefix == "" {
		return namespace + ":" + key
	}
	return store.prefix + ":" + namespace + ":" + key
}

func (store *firestoreDataStore) readExistingDocIDs(
	newData []ldstoretypes.SerializedCollection,
) (map[string]bool, error) {
	docIDs := make(map[string]bool)

	for _, coll := range newData {
		namespace := store.namespaceForKind(coll.Kind)
		query := store.client.Collection(store.collection).
			Where(fieldNamespace, "==", namespace).
			Select() // Select no fields, just get document IDs

		iter := query.Documents(store.context)
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				iter.Stop()
				return nil, err
			}
			docIDs[doc.Ref.ID] = true
		}
		iter.Stop()
	}

	return docIDs, nil
}

func (store *firestoreDataStore) decodeDocument(
	doc *firestore.DocumentSnapshot,
) (string, ldstoretypes.SerializedItemDescriptor, bool) {
	data := doc.Data()

	key, _ := data[fieldKey].(string)
	version, _ := data[fieldVersion].(int64)
	itemJSON, _ := data[fieldItem].(string)

	if key != "" {
		return key, ldstoretypes.SerializedItemDescriptor{
			Version:        int(version),
			SerializedItem: []byte(itemJSON),
		}, true
	}

	return "", ldstoretypes.SerializedItemDescriptor{}, false
}

func (store *firestoreDataStore) encodeItem(
	kind ldstoretypes.DataKind,
	key string,
	item ldstoretypes.SerializedItemDescriptor,
) map[string]any {
	return map[string]any{
		fieldNamespace: store.namespaceForKind(kind),
		fieldKey:       key,
		fieldVersion:   item.Version,
		fieldItem:      string(item.SerializedItem),
	}
}

func (store *firestoreDataStore) checkSizeLimit(data map[string]any) bool {
	// Rough estimate of document size
	size := 0
	for key, value := range data {
		size += len(key)
		if str, ok := value.(string); ok {
			size += len(str)
		} else {
			size += 8 // rough estimate for numeric values
		}
	}

	if size <= firestoreMaxDocSize {
		return true
	}

	store.loggers.Errorf("The item %q in namespace %q was too large to store in Firestore and was dropped",
		data[fieldKey], data[fieldNamespace])
	return false
}
