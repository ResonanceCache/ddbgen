//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ResonanceCache/ddbgen/runtime"
)

// The integration suite runs against DynamoDB Local:
//
//	make integration
//
// or with an explicit endpoint: DDB_TEST_ENDPOINT=http://localhost:8000
// go test -tags=integration ./examples/...
func newTestClient(t *testing.T) *AppClient {
	t.Helper()
	endpoint := os.Getenv("DDB_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("DDB_TEST_ENDPOINT not set; start DynamoDB Local with make up")
	}
	ddb := localDynamo(endpoint)
	table := fmt.Sprintf("app-it-%d", time.Now().UnixNano())
	ctx := context.Background()
	if err := createAppTable(ctx, ddb, table); err != nil {
		t.Fatal(err)
	}
	return NewAppClient(ddb, table)
}

var itT0 = time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

func seedOrder(t *testing.T, app *AppClient, id string, hours int, status string) *Order {
	t.Helper()
	o := &Order{
		TenantID:  "acme",
		OrderID:   id,
		Status:    status,
		Total:     100,
		CreatedAt: itT0.Add(time.Duration(hours) * time.Hour),
		UpdatedAt: itT0.Add(time.Duration(hours) * time.Hour),
	}
	if err := app.PutOrderIfNotExists(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	return o
}

func TestCRUDAndVersioning(t *testing.T) {
	app := newTestClient(t)
	ctx := context.Background()

	o := seedOrder(t, app, "o1", 1, "open")
	if o.Ver != 1 {
		t.Fatalf("PutOrderIfNotExists must set version 1, got %d", o.Ver)
	}
	dup := &Order{TenantID: "acme", OrderID: "o1", CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt}
	if err := app.PutOrderIfNotExists(ctx, dup); !errors.Is(err, runtime.ErrConditionFailed) {
		t.Fatalf("duplicate PutIfNotExists: want ErrConditionFailed, got %v", err)
	}

	got, err := app.GetOrder(ctx, "acme", o.CreatedAt, "o1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 100 || got.Ver != 1 {
		t.Fatalf("GetOrder: %+v", got)
	}

	// Optimistic locking: the stale copy must lose.
	stale := *got
	got.Total = 150
	if err := app.PutOrder(ctx, got); err != nil {
		t.Fatal(err)
	}
	if got.Ver != 2 {
		t.Fatalf("PutOrder must advance version in place, got %d", got.Ver)
	}
	stale.Total = 999
	if err := app.PutOrder(ctx, &stale); !errors.Is(err, runtime.ErrVersionConflict) {
		t.Fatalf("stale PutOrder: want ErrVersionConflict, got %v", err)
	}
	if back, _ := app.GetOrder(ctx, "acme", o.CreatedAt, "o1"); back.Total != 150 {
		t.Fatalf("stale write must not land, total=%d", back.Total)
	}

	// Typed update with version condition.
	upd, err := app.UpdateOrder("acme", o.CreatedAt, "o1").
		SetStatus("shipped").
		AddTotal(25).
		ExpectVersion(2).
		Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if upd.Status != "shipped" || upd.Total != 175 || upd.Ver != 3 {
		t.Fatalf("update result: %+v", upd)
	}
	if _, err := app.UpdateOrder("acme", o.CreatedAt, "o1").SetStatus("x").ExpectVersion(2).Run(ctx); !errors.Is(err, runtime.ErrVersionConflict) {
		t.Fatalf("stale update: want ErrVersionConflict, got %v", err)
	}
	if _, err := app.UpdateOrder("acme", itT0.Add(99*time.Hour), "ghost").SetStatus("x").Run(ctx); !errors.Is(err, runtime.ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}

	// The status setter resynced GSI1: the GSI pattern must find the item.
	found := 0
	for o, err := range app.OrdersByStatus("shipped").All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		if o.OrderID == "o1" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("OrdersByStatus after SetStatus: found %d", found)
	}

	if err := app.DeleteOrder(ctx, "acme", o.CreatedAt, "o1"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetOrder(ctx, "acme", o.CreatedAt, "o1"); !errors.Is(err, runtime.ErrNotFound) {
		t.Fatalf("after delete: want ErrNotFound, got %v", err)
	}
}

func TestPatternQueriesAndPagination(t *testing.T) {
	app := newTestClient(t)
	ctx := context.Background()

	// 1KB filler forces multiple pages under a small Limit and exercises
	// the internal pagination of All.
	filler := strings.Repeat("x", 1024)
	for i := 1; i <= 5; i++ {
		o := seedOrder(t, app, fmt.Sprintf("o%d", i), i, "open")
		if _, err := app.UpdateOrder("acme", o.CreatedAt, o.OrderID).SetNote(filler).Run(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Boundary cut: strictly-between excludes o1 (== lower bound is
	// inclusive) and o5 (outside).
	var ids []string
	for o, err := range app.OrdersByTenant("acme").
		CreatedBetween(itT0.Add(2*time.Hour), itT0.Add(4*time.Hour)).
		All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, o.OrderID)
	}
	if want := []string{"o2", "o3", "o4"}; !equal(ids, want) {
		t.Fatalf("CreatedBetween: got %v want %v", ids, want)
	}

	// CreatedAfter is strict; CreatedBefore is strict.
	ids = nil
	for o, err := range app.OrdersByTenant("acme").CreatedAfter(itT0.Add(3 * time.Hour)).All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, o.OrderID)
	}
	if want := []string{"o4", "o5"}; !equal(ids, want) {
		t.Fatalf("CreatedAfter: got %v want %v", ids, want)
	}
	ids = nil
	for o, err := range app.OrdersByTenant("acme").CreatedBefore(itT0.Add(3 * time.Hour)).Desc().All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, o.OrderID)
	}
	if want := []string{"o2", "o1"}; !equal(ids, want) {
		t.Fatalf("CreatedBefore Desc: got %v want %v", ids, want)
	}

	// All() with a tiny page limit must still stream everything.
	count := 0
	for _, err := range app.OrdersByTenant("acme").Limit(1).All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 5 {
		t.Fatalf("All with Limit(1): got %d items", count)
	}

	// Cursor pagination crosses pages.
	var cursor runtime.Cursor
	pages, items := 0, 0
	for {
		page, next, err := app.OrdersByTenant("acme").Limit(2).Page(ctx, cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		items += len(page)
		if next == "" {
			break
		}
		cursor = next
	}
	if items != 5 || pages < 3 {
		t.Fatalf("Page: %d items across %d pages", items, pages)
	}
}

func TestCollectionsAndBatch(t *testing.T) {
	app := newTestClient(t)
	ctx := context.Background()

	if err := app.PutTenantIfNotExists(ctx, &Tenant{TenantID: "acme", Name: "Acme", Plan: "pro"}); err != nil {
		t.Fatal(err)
	}
	o := seedOrder(t, app, "o1", 1, "open")
	for i := 1; i <= 3; i++ {
		p := &Payment{TenantID: "acme", OrderID: "o1", PaymentID: fmt.Sprintf("p%d", i), AmountCts: int64(i * 100)}
		if err := app.PutPayment(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	col, err := app.TenantPartition("acme").Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Tenants) != 1 || len(col.Orders) != 1 || len(col.Payments) != 3 || len(col.Unknown) != 0 {
		t.Fatalf("collection: %d tenants, %d orders, %d payments, %d unknown",
			len(col.Tenants), len(col.Orders), len(col.Payments), len(col.Unknown))
	}

	ids := []string{}
	for p, err := range app.PaymentsByOrder("acme", "o1").All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.PaymentID)
	}
	if want := []string{"p1", "p2", "p3"}; !equal(ids, want) {
		t.Fatalf("PaymentsByOrder: got %v want %v", ids, want)
	}

	// Batch write beyond one 25-item chunk, then batch read beyond nothing
	// (10 keys) and verify round trip.
	var batch []Order
	var keys []OrderKey
	for i := 0; i < 30; i++ {
		b := Order{
			TenantID:  "acme",
			OrderID:   fmt.Sprintf("bulk%02d", i),
			Status:    "open",
			Total:     int64(i),
			CreatedAt: itT0.Add(24*time.Hour + time.Duration(i)*time.Minute),
			UpdatedAt: itT0.Add(24 * time.Hour),
			Ver:       1,
		}
		batch = append(batch, b)
		keys = append(keys, OrderKey{TenantID: "acme", CreatedAt: b.CreatedAt, OrderID: b.OrderID})
	}
	if err := app.BatchPutOrders(ctx, batch); err != nil {
		t.Fatal(err)
	}
	got, err := app.BatchGetOrders(ctx, keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 30 {
		t.Fatalf("BatchGetOrders: got %d of 30", len(got))
	}
	// Missing keys are omitted, not errors.
	got, err = app.BatchGetOrders(ctx, []OrderKey{{TenantID: "acme", CreatedAt: itT0, OrderID: "ghost"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("BatchGetOrders missing: %d items, %v", len(got), err)
	}
	_ = o
}

func TestTransactWrite(t *testing.T) {
	app := newTestClient(t)
	ctx := context.Background()

	o := &Order{TenantID: "acme", OrderID: "o1", Status: "open", CreatedAt: itT0, UpdatedAt: itT0, Ver: 1}
	putOrder, err := app.TransactPutOrder(o)
	if err != nil {
		t.Fatal(err)
	}
	putPay, err := app.TransactPutPayment(&Payment{TenantID: "acme", OrderID: "o1", PaymentID: "p1", AmountCts: 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.TransactWrite(ctx, putOrder, putPay); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetOrder(ctx, "acme", itT0, "o1"); err != nil {
		t.Fatalf("order missing after transaction: %v", err)
	}
	if _, err := app.GetPayment(ctx, "acme", "o1", "p1"); err != nil {
		t.Fatalf("payment missing after transaction: %v", err)
	}

	del, err := app.TransactDeleteOrder("acme", itT0, "o1")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.TransactWrite(ctx, del); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetOrder(ctx, "acme", itT0, "o1"); !errors.Is(err, runtime.ErrNotFound) {
		t.Fatalf("order must be gone after transactional delete: %v", err)
	}
}

func TestDelimiterRejection(t *testing.T) {
	app := newTestClient(t)
	ctx := context.Background()
	err := app.PutTenantIfNotExists(ctx, &Tenant{TenantID: "bad#tenant"})
	if !errors.Is(err, runtime.ErrDelimiterInValue) {
		t.Fatalf("want ErrDelimiterInValue, got %v", err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
