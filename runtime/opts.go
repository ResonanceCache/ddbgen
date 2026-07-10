package runtime

// ReadOpt configures a generated Get or BatchGet call.
type ReadOpt func(*ReadOptions)

// ReadOptions is the resolved set of read options. Generated code reads it;
// applications construct it through ReadOpt values.
type ReadOptions struct {
	ConsistentRead bool
}

// WithConsistentRead makes the read strongly consistent (main index only;
// DynamoDB does not support consistent reads on GSIs).
func WithConsistentRead() ReadOpt {
	return func(o *ReadOptions) { o.ConsistentRead = true }
}

// ResolveReadOpts folds ReadOpt values into ReadOptions.
func ResolveReadOpts(opts []ReadOpt) ReadOptions {
	var o ReadOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// DeleteOpt configures a generated Delete call.
type DeleteOpt func(*DeleteOptions)

// DeleteOptions is the resolved set of delete options.
type DeleteOptions struct {
	ExpectVersion *int64
	MustExist     bool
}

// WithExpectVersion conditions the delete on the stored version being v;
// a mismatch (or a missing item) returns ErrVersionConflict.
func WithExpectVersion(v int64) DeleteOpt {
	return func(o *DeleteOptions) { o.ExpectVersion = &v }
}

// WithMustExist conditions the delete on the item existing; deleting a
// missing item returns ErrNotFound instead of succeeding silently.
func WithMustExist() DeleteOpt {
	return func(o *DeleteOptions) { o.MustExist = true }
}

// ResolveDeleteOpts folds DeleteOpt values into DeleteOptions.
func ResolveDeleteOpts(opts []DeleteOpt) DeleteOptions {
	var o DeleteOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
