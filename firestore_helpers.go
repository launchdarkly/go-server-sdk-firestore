package ldfirestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
)

// makeClientAndContext creates a Firestore client and context based on builder options
func makeClientAndContext(builder builderOptions) (*firestore.Client, context.Context, context.CancelFunc, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())

	client := builder.client
	if client == nil {
		var err error
		opts := builder.clientOptions
		if builder.projectID == "" {
			cancelFunc()
			return nil, nil, nil, fmt.Errorf("project ID is required")
		}
		client, err = firestore.NewClient(ctx, builder.projectID, opts...)
		if err != nil {
			cancelFunc()
			return nil, nil, nil, err
		}
	}

	return client, ctx, cancelFunc, nil
}

// batchWriteOperations executes a list of operations using Firestore's BulkWriter.
// BulkWriter automatically handles batching (up to 20 writes per batch) and sends
// operations in parallel for better performance.
func batchWriteOperations(
	ctx context.Context,
	client *firestore.Client,
	operations []firestoreOperation,
) error {
	bulkWriter := client.BulkWriter(ctx)

	// Enqueue all operations
	for _, op := range operations {
		if err := op.apply(bulkWriter); err != nil {
			return fmt.Errorf("failed to enqueue operation: %w", err)
		}
	}

	// Flush all operations and close the BulkWriter
	bulkWriter.End()

	return nil
}

// firestoreOperation represents a BulkWriter operation (set or delete)
type firestoreOperation interface {
	apply(bulkWriter *firestore.BulkWriter) error
}

// setOperation represents a set operation
type setOperation struct {
	ref  *firestore.DocumentRef
	data map[string]any
}

func (op setOperation) apply(bulkWriter *firestore.BulkWriter) error {
	_, err := bulkWriter.Set(op.ref, op.data)
	return err
}

// deleteOperation represents a delete operation
type deleteOperation struct {
	ref *firestore.DocumentRef
}

func (op deleteOperation) apply(bulkWriter *firestore.BulkWriter) error {
	_, err := bulkWriter.Delete(op.ref)
	return err
}
