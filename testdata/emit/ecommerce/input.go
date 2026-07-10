package ecommerce

import "time"

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
	CreatedAt time.Time `dynamodbav:"created_at"`
	UpdatedAt time.Time `dynamodbav:"updated_at"`
	Ver       int64     `dynamodbav:"v"`
	ExpiresAt int64     `dynamodbav:"exp,omitempty"`
}

//ddb:entity table=app type=payment
//ddb:key pk="TENANT#{TenantID}" sk="PAY#{OrderID}#{PaymentID}"
type Payment struct {
	TenantID  string `dynamodbav:"tenant_id"`
	OrderID   string `dynamodbav:"order_id"`
	PaymentID string `dynamodbav:"payment_id"`
	AmountCts int64  `dynamodbav:"amount_cts"`
}
