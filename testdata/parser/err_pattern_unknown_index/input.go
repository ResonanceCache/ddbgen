package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{ID}"
//ddb:pattern name=ByStatus index=GSI9 pk="STATUS#{Status}"
type Order struct {
	ID     string `dynamodbav:"id"`
	Status string `dynamodbav:"status"`
}
