package bounds

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ResonanceCache/ddbgen/runtime"
)

// modelDB implements runtime.DynamoDB over an in-memory item list with
// DynamoDB's lexicographic sort-key semantics, so generated range bounds
// are checked against ground truth instead of against their own strings.
type modelDB struct {
	items []map[string]types.AttributeValue
}

var errUnsupported = errors.New("modelDB: unsupported operation")

func s(av types.AttributeValue) string {
	if v, ok := av.(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

func (m *modelDB) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	names := in.ExpressionAttributeNames
	values := in.ExpressionAttributeValues
	cond := *in.KeyConditionExpression

	match := func(item map[string]types.AttributeValue) bool {
		if s(item[names["#pk"]]) != s(values[":pk"]) {
			return false
		}
		skCond, has := strings.CutPrefix(cond, "#pk = :pk")
		if !has || skCond == "" {
			return true
		}
		sk := s(item[names["#sk"]])
		a := s(values[":a"])
		switch {
		case skCond == " AND #sk = :a":
			return sk == a
		case skCond == " AND begins_with(#sk, :a)":
			return strings.HasPrefix(sk, a)
		case skCond == " AND #sk < :a":
			return sk < a
		case skCond == " AND #sk <= :a":
			return sk <= a
		case skCond == " AND #sk > :a":
			return sk > a
		case skCond == " AND #sk >= :a":
			return sk >= a
		case skCond == " AND #sk BETWEEN :a AND :b":
			b := s(values[":b"])
			if a > b {
				return false // DynamoDB would reject; the model just excludes
			}
			return sk >= a && sk <= b
		default:
			panic("modelDB: unrecognized key condition " + cond)
		}
	}

	filter := func(item map[string]types.AttributeValue) bool {
		if in.FilterExpression == nil {
			return true
		}
		for _, clause := range strings.Split(*in.FilterExpression, " AND ") {
			clause = strings.TrimSuffix(strings.TrimPrefix(clause, "("), ")")
			switch clause {
			case "#ddbet = :ddbet":
				if s(item[names["#ddbet"]]) != s(values[":ddbet"]) {
					return false
				}
			case "#sub = :sub":
				n, ok := item[names["#sub"]].(*types.AttributeValueMemberN)
				want := values[":sub"].(*types.AttributeValueMemberN)
				if !ok || n.Value != want.Value {
					return false
				}
			default:
				panic("modelDB: unrecognized filter clause " + clause)
			}
		}
		return true
	}

	var out []map[string]types.AttributeValue
	for _, item := range m.items {
		if match(item) && filter(item) {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return s(out[i]["sk"]) < s(out[j]["sk"])
	})
	if in.ScanIndexForward != nil && !*in.ScanIndexForward {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return &dynamodb.QueryOutput{Items: out}, nil
}

func (m *modelDB) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return nil, errUnsupported
}

func (m *modelDB) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return nil, errUnsupported
}

func (m *modelDB) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return nil, errUnsupported
}

func (m *modelDB) UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return nil, errUnsupported
}

func (m *modelDB) BatchGetItem(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	return nil, errUnsupported
}

func (m *modelDB) BatchWriteItem(context.Context, *dynamodb.BatchWriteItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	return nil, errUnsupported
}

func (m *modelDB) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return nil, errUnsupported
}

// seed builds one tenant partition holding every entity: jobs (seq 1..3 x
// sub 1..2), notes (seq 1..3, whose sort keys literally extend into the
// jobs' scope), and events (at 1..4, no literal scope at all).
func seed(t *testing.T) *BxClient {
	t.Helper()
	db := &modelDB{}
	add := func(av map[string]types.AttributeValue, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		db.items = append(db.items, av)
	}
	for seq := int64(1); seq <= 3; seq++ {
		for sub := int64(1); sub <= 2; sub++ {
			add(marshalJob(&Job{Ten: "t1", Seq: seq, Sub: sub}))
		}
		add(marshalNote(&Note{Ten: "t1", Seq: seq}))
	}
	for at := int64(1); at <= 4; at++ {
		add(marshalEvent(&Event{Ten: "t1", At: at}))
	}
	return NewBxClient(db, "bx")
}

func collectJobs(t *testing.T, it func(func(Job, error) bool)) []Job {
	t.Helper()
	var out []Job
	for j, err := range it {
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, j)
	}
	return out
}

// TestBoundsMatchSemantics checks every generated range cut against the
// semantic predicate it claims to implement.
func TestBoundsMatchSemantics(t *testing.T) {
	ctx := context.Background()
	c := seed(t)

	t.Run("begins without trailing delimiter selects all jobs and only jobs", func(t *testing.T) {
		jobs := collectJobs(t, c.JobsNoDelim("t1").All(ctx))
		if len(jobs) != 6 {
			t.Fatalf("want 6 jobs, got %d (notes leaked or jobs missed)", len(jobs))
		}
	})

	t.Run("SeqAfter is exact despite delimiter-less marker", func(t *testing.T) {
		jobs := collectJobs(t, c.JobsNoDelim("t1").SeqAfter(2).All(ctx))
		if len(jobs) != 2 {
			t.Fatalf("want 2 jobs with Seq > 2, got %d", len(jobs))
		}
		for _, j := range jobs {
			if j.Seq <= 2 {
				t.Fatalf("Seq %d leaked into After(2)", j.Seq)
			}
		}
	})

	t.Run("SeqBefore is exact, not partition-wide", func(t *testing.T) {
		jobs := collectJobs(t, c.JobsNoDelim("t1").SeqBefore(2).All(ctx))
		if len(jobs) != 2 {
			t.Fatalf("want 2 jobs with Seq < 2, got %d", len(jobs))
		}
		for _, j := range jobs {
			if j.Seq >= 2 {
				t.Fatalf("Seq %d leaked into Before(2)", j.Seq)
			}
		}
	})

	t.Run("SeqBetween is inclusive and entity-exact", func(t *testing.T) {
		jobs := collectJobs(t, c.JobsNoDelim("t1").SeqBetween(2, 3).All(ctx))
		if len(jobs) != 4 {
			t.Fatalf("want 4 jobs with Seq in [2,3], got %d", len(jobs))
		}
	})

	t.Run("deep placeholder-final prefix cuts correctly", func(t *testing.T) {
		jobs := collectJobs(t, c.JobsDeep("t1", 2).All(ctx))
		if len(jobs) != 2 {
			t.Fatalf("want the 2 jobs of seq 2, got %d", len(jobs))
		}
		jobs = collectJobs(t, c.JobsDeep("t1", 2).SubAfter(1).All(ctx))
		if len(jobs) != 1 || jobs[0].Sub != 2 {
			t.Fatalf("SubAfter(1) within seq 2: got %+v", jobs)
		}
	})

	t.Run("notes never absorb jobs though their keys extend the same scope", func(t *testing.T) {
		var notes []Note
		for n, err := range c.NotesByTen("t1").All(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			notes = append(notes, n)
		}
		if len(notes) != 3 {
			t.Fatalf("want 3 notes, got %d (jobs leaked in)", len(notes))
		}
	})

	t.Run("empty-scope ranges stay entity-exact in a shared partition", func(t *testing.T) {
		var events []Event
		for e, err := range c.Events("t1").AtAfter(2).All(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			events = append(events, e)
		}
		if len(events) != 2 {
			t.Fatalf("want events 3 and 4, got %d items", len(events))
		}
		for _, e := range events {
			if e.At <= 2 {
				t.Fatalf("At %d leaked into AtAfter(2)", e.At)
			}
		}
	})

	t.Run("reversed Between is empty, not an error", func(t *testing.T) {
		jobs := collectJobs(t, c.JobsNoDelim("t1").SeqBetween(3, 1).All(ctx))
		if len(jobs) != 0 {
			t.Fatalf("reversed bounds must select nothing, got %d", len(jobs))
		}
	})

	t.Run("user filter composes with the entity filter", func(t *testing.T) {
		jobs := collectJobs(t, c.JobsNoDelim("t1").Filter(
			"#sub = :sub",
			map[string]string{"#sub": "sub"},
			map[string]types.AttributeValue{":sub": &types.AttributeValueMemberN{Value: "1"}},
		).All(ctx))
		if len(jobs) != 3 {
			t.Fatalf("want the 3 sub=1 jobs, got %d", len(jobs))
		}
		for _, j := range jobs {
			if j.Sub != 1 {
				t.Fatalf("Sub %d slipped past the filter", j.Sub)
			}
		}
	})

	t.Run("collection dispatch separates all three entities", func(t *testing.T) {
		col, err := c.TenPartition("t1").Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(col.Jobs) != 6 || len(col.Notes) != 3 || len(col.Events) != 4 || len(col.Unknown) != 0 {
			t.Fatalf("collection: %d jobs, %d notes, %d events, %d unknown",
				len(col.Jobs), len(col.Notes), len(col.Events), len(col.Unknown))
		}
	})

	t.Run("runtime delimiter check refuses malformed tenants", func(t *testing.T) {
		q := c.JobsNoDelim("bad#tenant")
		for _, err := range q.All(ctx) {
			if !errors.Is(err, runtime.ErrDelimiterInValue) {
				t.Fatalf("want ErrDelimiterInValue, got %v", err)
			}
			return
		}
		t.Fatal("expected an error from the malformed tenant id")
	})
}
