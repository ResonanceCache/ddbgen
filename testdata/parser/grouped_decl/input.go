package grouped

type (
	// User lives in a grouped type declaration; the doc comment sits on
	// the TypeSpec rather than the GenDecl.
	//
	//ddb:entity table=app type=user et=kind
	//ddb:key pk="USER#{ID}" sk="PROFILE"
	User struct {
		ID    string `dynamodbav:"id"`
		Email string
		Note  string `dynamodbav:"-"`
	}

	Unrelated struct {
		X int
	}
)
