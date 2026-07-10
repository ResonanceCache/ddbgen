package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{Missing}" sk="NOTE#{Note}"
type Order struct {
	ID   string `dynamodbav:"id"`
	Note string `dynamodbav:"-"`
}
