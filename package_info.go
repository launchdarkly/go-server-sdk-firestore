// Package ldfirestore provides a Firestore-backed persistent data store for the LaunchDarkly Go SDK.
//
// For more details about how and why you can use a persistent data store, see:
// https://docs.launchdarkly.com/sdk/features/storing-data
//
// To use the Firestore data store with the LaunchDarkly client:
//
//     import ldfirestore "github.com/launchdarkly/go-server-sdk-firestore"
//
//     config := ld.Config{
//         DataStore: ldcomponents.PersistentDataStore(
//             ldfirestore.DataStore("my-project-id", "my-collection"),
//         ),
//     }
//     client, err := ld.MakeCustomClient("sdk-key", config, 5*time.Second)
//
// By default, the data store uses a basic Firestore client configuration that will
// use Application Default Credentials from the environment. If you want to customize
// the client, you can use the methods of the ldfirestore.StoreBuilder returned by
// ldfirestore.DataStore(). For example:
//
//     config := ld.Config{
//         DataStore: ldcomponents.PersistentDataStore(
//             ldfirestore.DataStore("my-project-id", "my-collection").
//                 Prefix("key-prefix"),
//         ).CacheSeconds(30),
//     }
//
// Note that CacheSeconds() is not a method of ldfirestore.StoreBuilder, but rather a method of
// ldcomponents.PersistentDataStore(), because the caching behavior is provided by the SDK for
// all database integrations.
//
// If you are also using Firestore for other purposes, the data store can coexist with
// other data in the same collection as long as you use the Prefix option to make each application
// use different keys. However, it is advisable to configure separate collections in Firestore, for
// better control over permissions and cost management.
package ldfirestore
