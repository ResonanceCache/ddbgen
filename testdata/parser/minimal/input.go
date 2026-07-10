package minimal

// Config is a single-item entity with a partition key only.
//
//ddb:entity table=cfg type=config
//ddb:key pk="CONFIG#{Name}"
type Config struct {
	Name  string `dynamodbav:"name"`
	Value string `dynamodbav:"value"`

	internal string
}
