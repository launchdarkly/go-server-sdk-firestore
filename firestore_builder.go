package ldfirestore

import (
	"cloud.google.com/go/firestore"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"google.golang.org/api/option"
)

// StoreBuilder is a builder for configuring the Firestore-backed persistent data store and/or Big
// Segment store.
//
// Both [DataStore] and [BigSegmentStore] return instances of this type. You can use methods of the
// builder to specify any non-default Firestore options you may want, before passing the builder to
// either [github.com/launchdarkly/go-server-sdk/v7/ldcomponents.PersistentDataStore] or
// [github.com/launchdarkly/go-server-sdk/v7/ldcomponents.BigSegments] as appropriate. The two types
// of stores are independent of each other; you do not need a Big Segment store if you are not using
// the Big Segments feature, and you do not need to use the same Firestore options for both.
//
// In this example, the main data store uses a Firestore collection called "launchdarkly", and the Big Segment
// store uses a Firestore collection called "launchdarkly-big-segments":
//
//	config.DataStore = ldcomponents.PersistentDataStore(
//		ldfirestore.DataStore("my-project", "launchdarkly"))
//	config.BigSegments = ldcomponents.BigSegments(
//		ldfirestore.BigSegmentStore("my-project", "launchdarkly-big-segments"))
//
// Note that the SDK also has its own options related to data storage that are configured
// at a different level, because they are independent of what database is being used. For
// instance, the builder returned by [github.com/launchdarkly/go-server-sdk/v7/ldcomponents.PersistentDataStore]
// has options for caching:
//
//	config.DataStore = ldcomponents.PersistentDataStore(
//		ldfirestore.DataStore("my-project", "launchdarkly"),
//	).CacheSeconds(15)
type StoreBuilder[T any] struct {
	builderOptions
	factory func(*StoreBuilder[T], subsystems.ClientContext) (T, error)
}

type builderOptions struct {
	client         *firestore.Client
	projectID      string
	collection     string
	prefix         string
	clientOptions  []option.ClientOption
}

// DataStore returns a configurable builder for a Firestore-backed data store.
//
// This is for the main data store that holds feature flag data. To configure a data store for
// Big Segments, use [BigSegmentStore] instead.
//
// The projectID parameter is the Google Cloud project ID, and collection is the name of the
// Firestore collection to use. Both parameters are required, and the collection must already
// exist in Firestore.
//
// You can use methods of the builder to specify any non-default Firestore options you may want,
// before passing the builder to [github.com/launchdarkly/go-server-sdk/v7/ldcomponents.PersistentDataStore].
// In this example, the store is configured to use a Firestore collection called "launchdarkly":
//
//	config.DataStore = ldcomponents.PersistentDataStore(
//		ldfirestore.DataStore("my-project", "launchdarkly"),
//	)
//
// Note that the SDK also has its own options related to data storage that are configured
// at a different level, because they are independent of what database is being used. For
// instance, the builder returned by [github.com/launchdarkly/go-server-sdk/v7/ldcomponents.PersistentDataStore]
// has options for caching:
//
//	config.DataStore = ldcomponents.PersistentDataStore(
//		ldfirestore.DataStore("my-project", "launchdarkly"),
//	).CacheSeconds(15)
func DataStore(projectID, collection string) *StoreBuilder[subsystems.PersistentDataStore] {
	return &StoreBuilder[subsystems.PersistentDataStore]{
		builderOptions: builderOptions{
			projectID:  projectID,
			collection: collection,
		},
		factory: createPersistentDataStore,
	}
}

// BigSegmentStore returns a configurable builder for a Firestore-backed Big Segment store.
//
// The projectID parameter is the Google Cloud project ID, and collection is the name of the
// Firestore collection to use. Both parameters are required, and the collection must already
// exist in Firestore.
//
// You can use methods of the builder to specify any non-default Firestore options you may want,
// before passing the builder to [github.com/launchdarkly/go-server-sdk/v7/ldcomponents.BigSegments].
// In this example, the store is configured to use a Firestore collection called "launchdarkly-big-segments":
//
//	config.BigSegments = ldcomponents.BigSegments(
//		ldfirestore.BigSegmentStore("my-project", "launchdarkly-big-segments"),
//	)
//
// Note that the SDK also has its own options related to Big Segments that are configured
// at a different level, because they are independent of what database is being used. For
// instance, the builder returned by [github.com/launchdarkly/go-server-sdk/v7/ldcomponents.BigSegments]
// has an option for the status polling interval:
//
//	config.BigSegments = ldcomponents.BigSegments(
//		ldfirestore.BigSegmentStore("my-project", "launchdarkly-big-segments"),
//	).StatusPollInterval(time.Second * 30)
func BigSegmentStore(projectID, collection string) *StoreBuilder[subsystems.BigSegmentStore] {
	return &StoreBuilder[subsystems.BigSegmentStore]{
		builderOptions: builderOptions{
			projectID:  projectID,
			collection: collection,
		},
		factory: createBigSegmentStore,
	}
}

// Prefix specifies a prefix for namespacing the data store's keys.
func (b *StoreBuilder[T]) Prefix(prefix string) *StoreBuilder[T] {
	b.prefix = prefix
	return b
}

// FirestoreClient specifies an existing Firestore client instance. Use this if you want to customize the client
// used by the data store in ways that are not supported by other StoreBuilder options. If you
// specify this option, then any configurations specified with ClientOptions will be ignored.
func (b *StoreBuilder[T]) FirestoreClient(client *firestore.Client) *StoreBuilder[T] {
	b.client = client
	return b
}

// ClientOptions specifies custom parameters for the firestore.NewClient client constructor. This can be used
// to set properties such as credentials programmatically, rather than relying on the defaults from the environment.
func (b *StoreBuilder[T]) ClientOptions(options ...option.ClientOption) *StoreBuilder[T] {
	b.clientOptions = options
	return b
}

// Build is called internally by the SDK.
func (b *StoreBuilder[T]) Build(context subsystems.ClientContext) (T, error) {
	return b.factory(b, context)
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *StoreBuilder[T]) DescribeConfiguration() ldvalue.Value {
	return ldvalue.String("Firestore")
}

func createPersistentDataStore(
	builder *StoreBuilder[subsystems.PersistentDataStore],
	clientContext subsystems.ClientContext,
) (subsystems.PersistentDataStore, error) {
	return newFirestoreDataStoreImpl(builder.builderOptions, clientContext.GetLogging().Loggers)
}

func createBigSegmentStore(
	builder *StoreBuilder[subsystems.BigSegmentStore],
	clientContext subsystems.ClientContext,
) (subsystems.BigSegmentStore, error) {
	return newFirestoreBigSegmentStoreImpl(builder.builderOptions, clientContext.GetLogging().Loggers)
}
