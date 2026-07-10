package bad

//ddb:entity table=app type=order
//ddb:key pk="TENANT#{TenantID}" sk="ORDER#{OrderID}"
//ddb:pattern name=Orders index=main pk="TENANT#{TenantID}" sk.begins="ORD"
type Order struct {
	TenantID string `dynamodbav:"tenant_id"`
	OrderID  string `dynamodbav:"order_id"`
}
