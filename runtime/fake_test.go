package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// fakeDB implements DynamoDB for unit tests: scripted responses per call.
type fakeDB struct {
	queryResponses []queryResponse
	queryCalls     int

	batchGetFn   func(*dynamodb.BatchGetItemInput) (*dynamodb.BatchGetItemOutput, error)
	batchGetIn   []*dynamodb.BatchGetItemInput
	batchWriteFn func(*dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error)
	batchWriteIn []*dynamodb.BatchWriteItemInput
}

type queryResponse struct {
	out *dynamodb.QueryOutput
	err error
}

var errFakeUnexpected = errors.New("fake: unexpected call")

func (f *fakeDB) Query(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if f.queryCalls >= len(f.queryResponses) {
		return nil, fmt.Errorf("%w: Query #%d", errFakeUnexpected, f.queryCalls+1)
	}
	r := f.queryResponses[f.queryCalls]
	f.queryCalls++
	return r.out, r.err
}

func (f *fakeDB) BatchGetItem(_ context.Context, in *dynamodb.BatchGetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	f.batchGetIn = append(f.batchGetIn, in)
	if f.batchGetFn == nil {
		return nil, errFakeUnexpected
	}
	return f.batchGetFn(in)
}

func (f *fakeDB) BatchWriteItem(_ context.Context, in *dynamodb.BatchWriteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	f.batchWriteIn = append(f.batchWriteIn, in)
	if f.batchWriteFn == nil {
		return nil, errFakeUnexpected
	}
	return f.batchWriteFn(in)
}

func (f *fakeDB) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return nil, errFakeUnexpected
}

func (f *fakeDB) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return nil, errFakeUnexpected
}

func (f *fakeDB) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return nil, errFakeUnexpected
}

func (f *fakeDB) UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return nil, errFakeUnexpected
}

func (f *fakeDB) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return nil, errFakeUnexpected
}
