package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/ResonanceCache/ddbgen/runtime"
)

// main runs the demo against DynamoDB Local (make demo): it creates a
// fresh table, seeds data, and exercises every generated method category.
func main() {
	ctx := context.Background()
	endpoint := os.Getenv("DDB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	ddb := localDynamo(endpoint)
	table := fmt.Sprintf("app-demo-%d", time.Now().UnixNano())
	must(createAppTable(ctx, ddb, table))
	fmt.Printf("table %s created against %s\n", table, endpoint)

	app := NewAppClient(ddb, table)
	t0 := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

	// --- put-if-not-exists ---
	must(app.PutTenantIfNotExists(ctx, &Tenant{TenantID: "acme", Name: "Acme Corp", Plan: "pro"}))
	if err := app.PutTenantIfNotExists(ctx, &Tenant{TenantID: "acme", Name: "Imposter"}); !errors.Is(err, runtime.ErrConditionFailed) {
		fatalf("expected ErrConditionFailed, got %v", err)
	}
	fmt.Println("PutTenantIfNotExists: second write correctly rejected")

	// --- create orders and payments ---
	for i := 1; i <= 3; i++ {
		o := &Order{
			TenantID:  "acme",
			OrderID:   fmt.Sprintf("o%d", i),
			Status:    "open",
			Total:     int64(100 * i),
			CreatedAt: t0.Add(time.Duration(i) * time.Hour),
			UpdatedAt: t0.Add(time.Duration(i) * time.Hour),
		}
		must(app.PutOrderIfNotExists(ctx, o))
	}
	must(app.PutPayment(ctx, &Payment{TenantID: "acme", OrderID: "o1", PaymentID: "p1", AmountCts: 5000}))
	must(app.PutPayment(ctx, &Payment{TenantID: "acme", OrderID: "o1", PaymentID: "p2", AmountCts: 2500}))
	fmt.Println("seeded 3 orders and 2 payments")

	// --- get ---
	o1, err := app.GetOrder(ctx, "acme", t0.Add(1*time.Hour), "o1")
	must(err)
	fmt.Printf("GetOrder: %s total=%d v=%d\n", o1.OrderID, o1.Total, o1.Ver)

	// --- optimistic locking: a stale writer loses ---
	stale := *o1
	o1.Total = 175
	must(app.PutOrder(ctx, o1))
	stale.Total = 999
	if err := app.PutOrder(ctx, &stale); !errors.Is(err, runtime.ErrVersionConflict) {
		fatalf("expected ErrVersionConflict, got %v", err)
	}
	fmt.Printf("PutOrder: stale write rejected with ErrVersionConflict (v=%d won)\n", o1.Ver)

	// --- typed update: SetStatus resyncs the GSI1 key automatically ---
	updated, err := app.UpdateOrder("acme", t0.Add(1*time.Hour), "o1").
		SetStatus("shipped").
		SetUpdatedAt(t0.Add(9 * time.Hour)).
		AddTotal(25).
		Run(ctx)
	must(err)
	fmt.Printf("UpdateOrder: status=%s total=%d v=%d\n", updated.Status, updated.Total, updated.Ver)

	// --- GSI pattern query finds the update without any manual index write ---
	shipped := 0
	for o, err := range app.OrdersByStatus("shipped").All(ctx) {
		must(err)
		shipped++
		fmt.Printf("OrdersByStatus(shipped): %s updated=%d\n", o.OrderID, o.UpdatedAt.Unix())
	}
	if shipped != 1 {
		fatalf("expected 1 shipped order, got %d", shipped)
	}

	// --- pattern query with a boundary cut, descending ---
	fmt.Println("OrdersByTenant CreatedBetween(t0+90m, t0+3h) Desc:")
	for o, err := range app.OrdersByTenant("acme").
		CreatedBetween(t0.Add(90*time.Minute), t0.Add(3*time.Hour)).
		Desc().
		All(ctx) {
		must(err)
		fmt.Printf("  %s created=%s\n", o.OrderID, o.CreatedAt.Format(time.RFC3339))
	}

	// --- cursor pagination ---
	var cursor runtime.Cursor
	pages := 0
	for {
		items, next, err := app.OrdersByTenant("acme").Limit(1).Page(ctx, cursor)
		must(err)
		pages++
		fmt.Printf("Page %d: %d order(s)\n", pages, len(items))
		if next == "" {
			break
		}
		cursor = next
	}

	// --- item collection: one query, typed dispatch ---
	col, err := app.TenantPartition("acme").Collect(ctx)
	must(err)
	fmt.Printf("TenantPartition: %d tenants, %d orders, %d payments, %d unknown\n",
		len(col.Tenants), len(col.Orders), len(col.Payments), len(col.Unknown))

	// --- batch: chunked writes and reads ---
	var batch []Order
	var keys []OrderKey
	for i := 0; i < 30; i++ {
		o := Order{
			TenantID:  "acme",
			OrderID:   fmt.Sprintf("bulk%02d", i),
			Status:    "open",
			Total:     int64(i),
			CreatedAt: t0.Add(24*time.Hour + time.Duration(i)*time.Minute),
			UpdatedAt: t0.Add(24 * time.Hour),
			Ver:       1,
		}
		batch = append(batch, o)
		keys = append(keys, OrderKey{TenantID: "acme", CreatedAt: o.CreatedAt, OrderID: o.OrderID})
	}
	must(app.BatchPutOrders(ctx, batch))
	got, err := app.BatchGetOrders(ctx, keys[:10])
	must(err)
	fmt.Printf("BatchPutOrders wrote 30 (chunked at 25); BatchGetOrders fetched %d\n", len(got))

	// --- payments by order ---
	for p, err := range app.PaymentsByOrder("acme", "o1").All(ctx) {
		must(err)
		fmt.Printf("PaymentsByOrder(o1): %s %d cts\n", p.PaymentID, p.AmountCts)
	}

	// --- delete ---
	must(app.DeleteOrder(ctx, "acme", t0.Add(3*time.Hour), "o3"))
	if _, err := app.GetOrder(ctx, "acme", t0.Add(3*time.Hour), "o3"); !errors.Is(err, runtime.ErrNotFound) {
		fatalf("expected ErrNotFound, got %v", err)
	}
	fmt.Println("DeleteOrder: o3 gone, Get returns ErrNotFound")
	fmt.Println("demo complete")
}

func localDynamo(endpoint string) *dynamodb.Client {
	return dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(endpoint),
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "local", SecretAccessKey: "local"}, nil
		}),
	})
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "demo: "+format+"\n", a...)
	os.Exit(1)
}
