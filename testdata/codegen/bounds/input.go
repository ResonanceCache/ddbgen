// Package bounds is the range-bound regression fixture: three entities
// share TEN# partitions in the most leak-prone legal shapes — a begins
// prefix written without its trailing delimiter, a deep prefix ending at a
// placeholder, an entity whose sort key literally extends a sibling's
// scope, and an entity with no literal scope at all. The paired harness
// runs the generated queries against an in-memory lexicographic model.
package bounds

//ddb:entity table=bx type=job
//ddb:key pk="TEN#{Ten}" sk="JOB#{Seq:pad4}#{Sub:pad4}"
//ddb:pattern name=JobsNoDelim index=main pk="TEN#{Ten}" sk.begins="JOB"
//ddb:pattern name=JobsDeep index=main pk="TEN#{Ten}" sk.begins="JOB#{Seq:pad4}"
type Job struct {
	Ten string `dynamodbav:"ten"`
	Seq int64  `dynamodbav:"seq"`
	Sub int64  `dynamodbav:"sub"`
}

//ddb:entity table=bx type=note
//ddb:key pk="TEN#{Ten}" sk="JOB#{Seq:pad4}"
//ddb:pattern name=NotesByTen index=main pk="TEN#{Ten}" sk.begins="JOB#"
type Note struct {
	Ten string `dynamodbav:"ten"`
	Seq int64  `dynamodbav:"seq"`
}

//ddb:entity table=bx type=event
//ddb:key pk="TEN#{Ten}" sk="{At:pad4}"
//ddb:pattern name=Events index=main pk="TEN#{Ten}" sk.between
type Event struct {
	Ten string `dynamodbav:"ten"`
	At  int64  `dynamodbav:"at"`
}
