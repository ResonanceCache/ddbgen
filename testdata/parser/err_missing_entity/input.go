package bad

//ddb:key pk="ORDER#{ID}"
type Order struct {
	ID string `dynamodbav:"id"`
}
