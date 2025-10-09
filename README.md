# LaunchDarkly Server-side SDK for Go - Firestore integration

[![Documentation](https://img.shields.io/static/v1?label=go.dev&message=reference&color=00add8)](https://pkg.go.dev/github.com/launchdarkly/go-server-sdk-firestore)

This library provides a [Google Cloud Firestore](https://cloud.google.com/firestore)-backed persistence mechanism (data store) for the [LaunchDarkly Go SDK](https://github.com/launchdarkly/go-server-sdk), replacing the default in-memory data store.

This version of the library requires at least version 7.0.0 of the LaunchDarkly Go SDK.

The minimum Go version is 1.23.

For more information, see also: [Using a persistent feature store](https://docs.launchdarkly.com/sdk/features/storing-data).

## Quick setup

This assumes that you have already installed the LaunchDarkly Go SDK.

1. Import the LaunchDarkly SDK packages and the package for this library:

```go
import (
    ld "github.com/launchdarkly/go-server-sdk/v7"
    "github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
    ldfirestore "github.com/launchdarkly/go-server-sdk-firestore"
)
```

2. When configuring your SDK client, add the Firestore data store as a `PersistentDataStore`. You must specify your Google Cloud project ID and the Firestore collection name. You may also specify any custom Firestore options using the methods of `StoreBuilder`. For instance:

```go
    var config ld.Config{}
    config.DataStore = ldcomponents.PersistentDataStore(
        ldfirestore.DataStore("my-project-id", "launchdarkly"),
    )
```

By default, the Firestore client will use [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials) to authenticate with Google Cloud. You can customize the authentication by using the `ClientOptions` method:

```go
    import "google.golang.org/api/option"

    config.DataStore = ldcomponents.PersistentDataStore(
        ldfirestore.DataStore("my-project-id", "launchdarkly").
            ClientOptions(option.WithCredentialsFile("/path/to/credentials.json")),
    )
```

## Caching behavior

The LaunchDarkly SDK has a standard caching mechanism for any persistent data store, to reduce database traffic. This is configured through the SDK's `PersistentDataStoreBuilder` class as described in the SDK documentation. For instance, to specify a cache TTL of 5 minutes:

```go
    var config ld.Config{}
    config.DataStore = ldcomponents.PersistentDataStore(
        ldfirestore.DataStore("my-project-id", "launchdarkly"),
    ).CacheMinutes(5)
```

## Data size limitation

Firestore has [a 1 MiB limit](https://cloud.google.com/firestore/quotas) on the size of any document. For the LaunchDarkly SDK, a document consists of the JSON representation of an individual feature flag or segment configuration, plus a few smaller attributes. You can see the format and size of these representations by querying `https://sdk.launchdarkly.com/flags/latest-all` and setting the `Authorization` header to your SDK key.

Most flags and segments won't be nearly as big as 1 MiB, but they could be if for instance you have a long list of user keys for individual user targeting. If the flag or segment representation is too large, it cannot be stored in Firestore. To avoid disrupting storage and evaluation of other unrelated feature flags, the SDK will simply skip storing that individual flag or segment, and will log a message (at ERROR level) describing the problem. For example:

```
    The item "my-flag-key" in namespace "features" was too large to store in Firestore and was dropped
```

This implementation uses a conservative limit of ~900 KB to account for field overhead and indexing. If caching is enabled in your configuration, the flag or segment may still be available in the SDK from the in-memory cache, but do not rely on this. If you see this message, consider redesigning your flag/segment configurations, or else do not use Firestore for the environment that contains this data item.

[Big Segments](https://docs.launchdarkly.com/home/users/big-segments/) are much less likely to encounter this limitation because they distribute data differently: instead of storing all user memberships in a single segment document, Big Segments store one document per user containing only the segment keys they belong to. This means a segment with 100,000 users results in 100,000 small documents rather than one large document. The size limit still technically applies to Big Segment documents, but it would only be reached if a single user belonged to an extremely large number of segments (thousands), which is rare in practice.

## Using the Firestore Emulator for development

For local development and testing, you can use the [Firestore Emulator](https://cloud.google.com/firestore/docs/emulator):

```bash
# Start the emulator
gcloud emulators firestore start --host-port=localhost:8080

# Set the environment variable
export FIRESTORE_EMULATOR_HOST=localhost:8080

# Run your application or tests
go test ./...
```

When the `FIRESTORE_EMULATOR_HOST` environment variable is set, the Firestore client will automatically connect to the emulator instead of the production Firestore service.

## LaunchDarkly overview

[LaunchDarkly](https://www.launchdarkly.com) is a feature management platform that serves trillions of feature flags daily to help teams build better software, faster. [Get started](https://docs.launchdarkly.com/docs/getting-started) using LaunchDarkly today!

## About LaunchDarkly

* LaunchDarkly is a continuous delivery platform that provides feature flags as a service and allows developers to iterate quickly and safely. We allow you to easily flag your features and manage them from the LaunchDarkly dashboard.  With LaunchDarkly, you can:
    * Roll out a new feature to a subset of your users (like a group of users who opt-in to a beta tester group), gathering feedback and bug reports from real-world use cases.
    * Gradually roll out a feature to an increasing percentage of users, and track the effect that the feature has on key metrics (for instance, how likely is a user to complete a purchase if they have feature A versus feature B?).
    * Turn off a feature that you realize is causing performance problems in production, without needing to re-deploy, or even restart the application with a changed configuration file.
    * Grant access to certain features based on user attributes, like payment plan (eg: users on the 'gold' plan get access to more features than users in the 'silver' plan). Disable parts of your application to facilitate maintenance, without taking everything offline.
* LaunchDarkly provides feature flag SDKs for a wide variety of languages and technologies. Read [our documentation](https://docs.launchdarkly.com/sdk) for a complete list.
* Explore LaunchDarkly
    * [launchdarkly.com](https://www.launchdarkly.com/ "LaunchDarkly Main Website") for more information
    * [docs.launchdarkly.com](https://docs.launchdarkly.com/  "LaunchDarkly Documentation") for our documentation and SDK reference guides
    * [apidocs.launchdarkly.com](https://apidocs.launchdarkly.com/  "LaunchDarkly API Documentation") for our API documentation
    * [blog.launchdarkly.com](https://blog.launchdarkly.com/  "LaunchDarkly Blog Documentation") for the latest product updates
