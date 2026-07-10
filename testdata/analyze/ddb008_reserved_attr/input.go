package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{ID}" sk="A#{ID}"
//ddb:index name=GSI1 pk="B#{Kind}" sk="C#{ID}"
type Order struct {
	ID     string `dynamodbav:"id"`
	Kind   string `dynamodbav:"kind"`
	Legacy string `dynamodbav:"pk"`
	Shadow string `dynamodbav:"gsi1sk"`
	Tag    string `dynamodbav:"_et"`
	Alias  string `dynamodbav:"kind"`
}
