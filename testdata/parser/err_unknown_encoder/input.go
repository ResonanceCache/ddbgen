package bad

import "time"

//ddb:entity table=app type=order
//ddb:key pk="TENANT#{TenantID}" sk="ORDER#{CreatedAt:rfc9999}"
type Order struct {
	TenantID  string    `dynamodbav:"tenant_id"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}
