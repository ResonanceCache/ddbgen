package bad

//ddb:entity table=app type=order
//ddb:key pk="ORD{ID}"
type Order struct {
	ID string `dynamodbav:"id"`
}
