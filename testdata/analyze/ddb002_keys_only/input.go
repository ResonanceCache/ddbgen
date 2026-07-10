package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{ID}"
//ddb:index name=GSI1 pk="K#{Kind}" sk="{ID}" projection=keys_only
//ddb:pattern name=ByKind index=GSI1 pk="K#{Kind}"
type Order struct {
	ID   string `dynamodbav:"id"`
	Kind string `dynamodbav:"kind"`
}
