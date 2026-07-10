// The ecommerce example models a multi-tenant order system on one
// DynamoDB table ("app"): tenants, their orders, and order payments share
// tenant partitions as an item collection, and orders are additionally
// indexed by status on GSI1.
//
// Regenerate with:
//
//	ddbgen generate ./examples/ecommerce
//	ddbgen docs ./examples/ecommerce
//	ddbgen infra --format cfn --out examples/ecommerce/infra ./examples/ecommerce
package main

import "time"

// Order is one customer order. Orders live in their tenant's partition,
// sorted by creation time, and are queryable by status via GSI1. The Ver
// field enables optimistic locking; ExpiresAt drives table TTL.
//
//ddb:entity table=app type=order version=Ver ttl=ExpiresAt
//ddb:key pk="TENANT#{TenantID}" sk="ORDER#{CreatedAt:rfc3339}#{OrderID}"
//ddb:index name=GSI1 pk="STATUS#{Status:upper}" sk="{UpdatedAt:epoch}"
//ddb:pattern name=OrdersByTenant index=main pk="TENANT#{TenantID}" sk.begins="ORDER#"
//ddb:pattern name=OrdersByStatus index=GSI1 pk="STATUS#{Status:upper}"
type Order struct {
	TenantID  string    `dynamodbav:"tenant_id"`
	OrderID   string    `dynamodbav:"order_id"`
	Status    string    `dynamodbav:"status"`
	Total     int64     `dynamodbav:"total"`
	Note      string    `dynamodbav:"note,omitempty"`
	CreatedAt time.Time `dynamodbav:"created_at"`
	UpdatedAt time.Time `dynamodbav:"updated_at"`
	Ver       int64     `dynamodbav:"v"`
	ExpiresAt int64     `dynamodbav:"exp,omitempty"`
}

// Payment is one payment against an order, stored in the same tenant
// partition as the order it pays for.
//
//ddb:entity table=app type=payment
//ddb:key pk="TENANT#{TenantID}" sk="PAY#{OrderID}#{PaymentID}"
//ddb:pattern name=PaymentsByOrder index=main pk="TENANT#{TenantID}" sk.begins="PAY#{OrderID}#"
type Payment struct {
	TenantID  string `dynamodbav:"tenant_id"`
	OrderID   string `dynamodbav:"order_id"`
	PaymentID string `dynamodbav:"payment_id"`
	AmountCts int64  `dynamodbav:"amount_cts"`
}

// Tenant is the partition's metadata item.
//
//ddb:entity table=app type=tenant
//ddb:key pk="TENANT#{TenantID}" sk="TENANT#{TenantID}"
type Tenant struct {
	TenantID string `dynamodbav:"tenant_id"`
	Name     string `dynamodbav:"name"`
	Plan     string `dynamodbav:"plan"`
}
