package bad

//ddb:entity table=app type=box
//ddb:key pk="BOX#{ID}"
type Box[T any] struct {
	ID    string `dynamodbav:"id"`
	Value T      `dynamodbav:"value"`
}
